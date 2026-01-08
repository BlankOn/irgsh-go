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
	activeTasks int = 0
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	var err error
	irgshConfig, err = config.LoadConfig()
	if err != nil {
		log.Fatalln("couldn't load config : ", err)
	}
	// Prepare workdir
	err = os.MkdirAll(irgshConfig.Repo.Workdir, 0755)
	if err != nil {
		log.Fatalln(err)
	}

	err = os.MkdirAll(irgshConfig.Repo.Workdir, 0755)
	if err != nil {
		log.Fatalln(err)
	}

	app = cli.NewApp()
	app.Name = "irgsh-go"
	app.Usage = "irgsh-go distributed packager"
	app.Author = "BlankOn Developer"
	app.Email = "blankon-dev@googlegroups.com"
	app.Version = version

	app.Commands = []cli.Command{
		{
			Name:    "init",
			Aliases: []string{"i"},
			Usage:   "initialize repository",
			Action: func(c *cli.Context) (err error) {
				err = InitRepo()
				return err
			},
		},
		{
			Name:    "sync",
			Aliases: []string{"i"},
			Usage:   "update base.tgz",
			Action: func(c *cli.Context) (err error) {
				err = UpdateRepo()
				return err
			},
		},
	}

	app.Action = func(c *cli.Context) error {

		go serve()

		// Start monitoring heartbeat if enabled
		if irgshConfig.Monitoring.Enabled {
			go startMonitoringHeartbeat()
		}

		server, err := machinery.NewServer(
			&machineryConfig.Config{
				Broker:        irgshConfig.Redis,
				ResultBackend: irgshConfig.Redis,
				DefaultQueue:  "irgsh",
			},
		)
		if err != nil {
			fmt.Println("Could not create server : " + err.Error())
		}

		// Wrap Repo task with monitoring
		server.RegisterTask("repo", RepoWithMonitoring)
		// One worker for synchronous
		worker := server.NewWorker("repo", 1)
		err = worker.Launch()
		if err != nil {
			fmt.Println("Could not launch worker : " + err.Error())
		}
		return nil

	}
	app.Run(os.Args)
}

// RepoWithMonitoring wraps the Repo function with active task tracking
func RepoWithMonitoring(payload string) error {
	// Increment active tasks
	activeTasks++
	defer func() { activeTasks-- }()

	// Call original Repo function
	return Repo(payload)
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
	instanceID := monitoring.GenerateInstanceID(monitoring.InstanceTypeRepo)
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
	metrics := monitoring.CollectMetrics(irgshConfig.Repo.Workdir)

	// Build instance info
	instance := monitoring.InstanceInfo{
		InstanceID:    instanceID,
		InstanceType:  monitoring.InstanceTypeRepo,
		Hostname:      monitoring.GetHostname(),
		PID:           os.Getpid(),
		StartTime:     startTime,
		LastHeartbeat: time.Now(),
		Status:        monitoring.StatusOnline,
		Concurrency:   1, // Repo worker runs with concurrency 1
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
	fmt.Fprintf(w, "irgsh-repo "+app.Version)
}

func serve() {
	http.HandleFunc("/", IndexHandler)
	http.Handle("/dev/",
		http.StripPrefix("/dev/",
			http.FileServer(
				http.Dir(irgshConfig.Repo.Workdir+"/"+irgshConfig.Repo.DistCodename+"/www"),
			),
		),
	)
	http.Handle("/experimental/",
		http.StripPrefix("/experimental/",
			http.FileServer(
				http.Dir(irgshConfig.Repo.Workdir+"/"+irgshConfig.Repo.DistCodename+"-experimental/www"),
			),
		),
	)
	port := os.Getenv("PORT")
	if len(port) < 1 {
		port = "8082"
	}
	log.Println("irgsh-go repo is now live on port " + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
