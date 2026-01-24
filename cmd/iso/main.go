package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	machinery "github.com/RichardKnop/machinery/v1"
	machineryConfig "github.com/RichardKnop/machinery/v1/config"
	"github.com/urfave/cli"

	"github.com/blankon/irgsh-go/internal/config"
	"github.com/blankon/irgsh-go/internal/monitoring"
)

var (
	app        *cli.App
	configPath string
	server     *machinery.Server
	version    string

	irgshConfig = config.IrgshConfig{}

	// Monitoring
	activeTasks int = 0
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	app = cli.NewApp()
	app.Name = "irgsh-go"
	app.Usage = "irgsh-go distributed ISO builder"
	app.Author = "BlankOn Developer"
	app.Email = "blankon-dev@googlegroups.com"
	app.Version = version

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "config, c",
			Usage:       "Path to config file (optional, will use default paths if not specified)",
			Destination: &configPath,
		},
	}

	app.Before = func(c *cli.Context) error {
		var err error
		if configPath != "" {
			irgshConfig, err = config.LoadConfigFromPath(configPath)
		} else {
			irgshConfig, err = config.LoadConfig()
		}
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("Error: couldn't load config: %v", err), 1)
		}

		// Prepare workdir
		err = os.MkdirAll(irgshConfig.ISO.Workdir, 0755)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("Error: couldn't create workdir: %v", err), 1)
		}

		return nil
	}

	app.Commands = []cli.Command{
		{
			Name:    "init",
			Aliases: []string{"i"},
			Usage:   "Initialize ISO builder",
			Action: func(c *cli.Context) error {
				// Placeholder for initialization
				fmt.Println("ISO builder initialized")
				return nil
			},
		},
	}

	app.Action = func(c *cli.Context) error {

		go serve()

		// Start monitoring heartbeat if enabled
		if irgshConfig.Monitoring.Enabled {
			go startMonitoringHeartbeat()
		}

		var err error
		server, err = machinery.NewServer(
			&machineryConfig.Config{
				Broker:        irgshConfig.Redis,
				ResultBackend: irgshConfig.Redis,
				DefaultQueue:  "irgsh",
			},
		)
		if err != nil {
			fmt.Println("Could not create server : " + err.Error())
		}

		// Register ISO build task with monitoring wrapper
		server.RegisterTask("iso", ISOBuildWithMonitoring)

		worker := server.NewWorker("iso", 1)
		err = worker.Launch()
		if err != nil {
			fmt.Println("Could not launch worker : " + err.Error())
		}

		return nil

	}
	app.Run(os.Args)
}

// ISOBuildWithMonitoring wraps the BuildISO function with active task tracking
func ISOBuildWithMonitoring(payload string) (string, error) {
	// Increment active tasks
	activeTasks++
	defer func() { activeTasks-- }()

	// Call original BuildISO function
	return BuildISO(payload)
}

// startMonitoringHeartbeat sends periodic heartbeats to Redis
func startMonitoringHeartbeat() {
	// Create registry client
	ttl := time.Duration(irgshConfig.Monitoring.InstanceTimeout) * time.Second
	registry, err := monitoring.NewRegistry(irgshConfig.Redis, ttl)
	if err != nil {
		log.Printf("Failed to create monitoring registry: %v\n", err)
		return
	}
	defer registry.Close()

	// Generate instance ID
	instanceID := monitoring.GenerateInstanceID(monitoring.InstanceTypeISO)
	startTime := time.Now()

	interval := time.Duration(irgshConfig.Monitoring.HeartbeatInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("Monitoring heartbeat started (instance: %s, interval: %v)\n", instanceID, interval)

	// Send initial heartbeat
	sendHeartbeat(registry, instanceID, startTime)

	// Send periodic heartbeats
	for range ticker.C {
		sendHeartbeat(registry, instanceID, startTime)
	}
}

// sendHeartbeat collects metrics and sends them to Redis
func sendHeartbeat(registry *monitoring.Registry, instanceID string, startTime time.Time) {
	// Collect system metrics
	metrics := monitoring.CollectMetrics(irgshConfig.ISO.Workdir)

	// Build instance info
	instance := monitoring.InstanceInfo{
		InstanceID:    instanceID,
		InstanceType:  monitoring.InstanceTypeISO,
		Hostname:      monitoring.GetHostname(),
		PID:           os.Getpid(),
		StartTime:     startTime,
		LastHeartbeat: time.Now(),
		Status:        monitoring.StatusOnline,
		Concurrency:   1, // ISO worker runs with concurrency 1
		ActiveTasks:   activeTasks,
		CPUUsage:      metrics.CPUUsage,
		MemoryUsage:   metrics.MemoryUsage,
		MemoryTotal:   metrics.MemoryTotal,
		DiskUsage:     metrics.DiskUsage,
		DiskTotal:     metrics.DiskTotal,
		Version:       monitoring.GetVersion(),
	}

	// Write to Redis
	if err := registry.UpdateInstance(instance); err != nil {
		log.Printf("Failed to send heartbeat: %v\n", err)
	}
}

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "irgsh-iso "+app.Version)
}

func serve() {
	http.HandleFunc("/", IndexHandler)
	http.Handle("/artifacts/",
		http.StripPrefix("/artifacts/",
			http.FileServer(http.Dir(irgshConfig.ISO.Workdir+"/artifacts")),
		),
	)
	port := os.Getenv("PORT")
	if len(port) < 1 {
		port = "8083"
	}
	log.Println("irgsh-go iso now live on port " + port + ", serving path: " + irgshConfig.ISO.Workdir)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
