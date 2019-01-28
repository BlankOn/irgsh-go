package main

import (
	"fmt"
	machinery "github.com/RichardKnop/machinery/v1"
	"github.com/RichardKnop/machinery/v1/config"
	"github.com/urfave/cli"
	"os"
)

var (
	app        *cli.App
	configPath string
	server     *machinery.Server
	workdir    string
)

func loadConfig() (*config.Config, error) {
	if configPath != "" {
		return config.NewFromYaml(configPath, true)
	}
	return config.NewFromEnvironment(true)
}

func main() {
	app = cli.NewApp()
	app.Name = "irgsh-go"
	app.Usage = "irgsh-go distributed packager"
	app.Author = "BlankOn Developer"
	app.Email = "blankon-dev@googlegroups.com"
	app.Version = "0.0.1"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "c",
			Value:       "",
			Destination: &configPath,
			Usage:       "Path to a configuration file",
		},
	}

	app.Action = func(c *cli.Context) error {

		conf, err := loadConfig()
		if err != nil {
			fmt.Println("Failed to load : " + err.Error())
		}

		// IRGSH related config from ENV
		workdir = os.Getenv("WORKDIR")
		if len(workdir) == 0 {
			workdir = "/tmp"
		}

		server, err = machinery.NewServer(conf)
		if err != nil {
			fmt.Println("Could not create server : " + err.Error())
		}

		server.RegisterTask("clone", Clone)

		worker := server.NewWorker("builder", 2)
		err = worker.Launch()
		if err != nil {
			fmt.Println("Could not launch worker : " + err.Error())
		}
		return nil

	}
	app.Run(os.Args)
}
