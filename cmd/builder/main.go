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
	var err error
	irgshConfig, err = config.LoadConfig()
	if err != nil {
		log.Fatalln("couldn't load config : ", err)
	}
	// Prepare workdir
	err = os.MkdirAll(irgshConfig.Builder.Workdir, 0755)
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

		go serve()

		// Start monitoring heartbeat if enabled
		if irgshConfig.Monitoring.Enabled {
			go startMonitoringHeartbeat()
		}

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
