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
	app.Usage = "irgsh-go distributed packager"
	app.Author = "BlankOn Developer"
	app.Email = "blankon-dev@googlegroups.com"
	app.Version = version

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "config, c",
			Usage:       "Path to config file (required)",
			Destination: &configPath,
		},
	}

	app.Before = func(c *cli.Context) error {

		// Config path is required for irgsh-repo for two reasons:
		// 1. We will let multiple irgsh-repo instances to be run in a single machine
		//    to handle multiple architectures
		// 2. Because of the nature of multiple configurations, it's a bit dangerous to mix them.
		//    So each instance will have its own configuration.
		if configPath == "" {
			return cli.NewExitError("Error: config path is required. Use -c or --config to specify the config file path.\n\nExample: irgsh-repo -c /path/to/config.yaml", 1)
		}

		var err error
		irgshConfig, err = config.LoadConfigFromPath(configPath)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("Error: couldn't load config: %v", err), 1)
		}

		// Prepare workdir
		err = os.MkdirAll(irgshConfig.Repo.Workdir, 0755)
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("Error: couldn't create workdir: %v", err), 1)
		}

		return nil
	}

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
	activeTasks.Add(1)
	defer activeTasks.Add(-1)

	return Repo(payload)
}

func startMonitoringHeartbeat() {
	ttl := time.Duration(irgshConfig.Monitoring.InstanceTimeout) * time.Second
	interval := time.Duration(irgshConfig.Monitoring.HeartbeatInterval) * time.Second
	monitoring.StartHeartbeatLoop(
		context.Background(),
		irgshConfig.Redis, ttl,
		monitoring.InstanceTypeRepo, irgshConfig.Repo.Workdir,
		interval, func() int { return int(activeTasks.Load()) },
	)
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
