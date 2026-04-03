package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
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

	activeTasks atomic.Int32
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
	activeTasks.Add(1)
	defer activeTasks.Add(-1)

	return BuildISO(payload)
}

func startMonitoringHeartbeat() {
	ttl := time.Duration(irgshConfig.Monitoring.InstanceTimeout) * time.Second
	interval := time.Duration(irgshConfig.Monitoring.HeartbeatInterval) * time.Second
	monitoring.StartHeartbeatLoop(
		context.Background(),
		irgshConfig.Redis, ttl,
		monitoring.InstanceTypeISO, irgshConfig.ISO.Workdir,
		interval, func() int { return int(activeTasks.Load()) },
	)
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
