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
	"github.com/blankon/irgsh-go/internal/logstream"
	"github.com/blankon/irgsh-go/internal/monitoring"
)

var (
	app        *cli.App
	configPath string
	server     *machinery.Server
	version    string

	irgshConfig = config.IrgshConfig{}

	activeTasks atomic.Int32

	// logPublisher mirrors job logs to chief while a job is running. It stays
	// nil when Redis is unreachable: live streaming is an addition to the log
	// file, never a reason to fail a build.
	logPublisher *logstream.Publisher
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
			Usage:       "Path to config file. Defaults to the usual search path, starting with /etc/irgsh/config.yaml",
			Destination: &configPath,
		},
	}

	app.Before = func(c *cli.Context) error {
		var err error
		// Without -c we keep the historical search path
		// (/etc/irgsh/config.yaml first); with it, that one file is the
		// config, so several builders can run side by side on one machine.
		if configPath == "" {
			irgshConfig, err = config.LoadConfig(config.ComponentBuilder)
		} else {
			irgshConfig, err = config.LoadConfigFromPath(configPath, config.ComponentBuilder)
		}
		if err != nil {
			return cli.NewExitError(fmt.Sprintf("Error: couldn't load config: %v", err), 1)
		}

		// Config validation is scoped to this component's own section, so the
		// chief address is not covered by it - but logs and artifacts go there.
		if irgshConfig.Chief.Address == "" {
			return cli.NewExitError("Error: chief.address is required so the worker can upload logs and artifacts to chief", 1)
		}

		// Prepare workdir
		if err = os.MkdirAll(irgshConfig.Builder.Workdir, 0755); err != nil {
			return cli.NewExitError(fmt.Sprintf("Error: couldn't create workdir: %v", err), 1)
		}

		logPublisher, err = logstream.NewPublisher(irgshConfig.Redis)
		if err != nil {
			log.Printf("live log streaming disabled: %v\n", err)
			logPublisher = nil
		}

		return nil
	}

	app.Commands = []cli.Command{
		{
			Name:    "init-builder",
			Aliases: []string{"i"},
			Usage:   "Initialize builder",
			Action: func(c *cli.Context) error {
				err := InitBuilder()
				return err
			},
		},
		{
			Name:    "init-base",
			Aliases: []string{"i"},
			Usage:   "Initialize pbuilder base.tgz. This need to be run under sudo or root",
			Action: func(c *cli.Context) error {
				err := InitBase()
				return err
			},
		},
		{
			Name:    "update-base",
			Aliases: []string{"i"},
			Usage:   "update base.tgz",
			Action: func(c *cli.Context) error {
				err := UpdateBase()
				return err
			},
		},
	}

	app.Action = func(c *cli.Context) error {
		var err error

		go serve()

		// Start monitoring heartbeat if enabled
		if irgshConfig.Monitoring.Enabled {
			go startMonitoringHeartbeat()
		}

		server, err = machinery.NewServer(
			&machineryConfig.Config{
				Broker:        irgshConfig.Redis,
				ResultBackend: irgshConfig.Redis,
				DefaultQueue:  config.DistQueue(irgshConfig.Builder.DistCodename),
			},
		)
		if err != nil {
			fmt.Println("Could not create server : " + err.Error())
		}

		// Wrap Build task with monitoring
		server.RegisterTask("build", BuildWithMonitoring)

		worker := server.NewWorker("builder", 1)
		err = worker.Launch()
		if err != nil {
			fmt.Println("Could not launch worker : " + err.Error())
		}

		return nil

	}
	app.Run(os.Args)
}

// BuildWithMonitoring wraps the Build function with active task tracking
func BuildWithMonitoring(payload string) (string, error) {
	activeTasks.Add(1)
	defer activeTasks.Add(-1)

	return Build(payload)
}

func startMonitoringHeartbeat() {
	ttl := time.Duration(irgshConfig.Monitoring.InstanceTimeout) * time.Second
	interval := time.Duration(irgshConfig.Monitoring.HeartbeatInterval) * time.Second
	monitoring.StartHeartbeatLoop(
		context.Background(),
		irgshConfig.Redis, ttl,
		monitoring.InstanceTypeBuilder, irgshConfig.Builder.Workdir,
		irgshConfig.Builder.DistCodename, monitoring.RepoHeartbeatInfo{},
		interval, func() int { return int(activeTasks.Load()) },
	)
}

func serve() {
	port := os.Getenv("PORT")
	if len(port) < 1 {
		port = "8081"
	}
	fs := http.FileServer(http.Dir(irgshConfig.Builder.Workdir))
	http.Handle("/", fs)
	log.Println("irgsh-go builder now live on port " + port + ", serving path : " + irgshConfig.Builder.Workdir)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
