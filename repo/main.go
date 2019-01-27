package main

import (
	"fmt"
	"os"

	machinery "github.com/RichardKnop/machinery/v1"
	"github.com/RichardKnop/machinery/v1/config"
	"github.com/urfave/cli"
)

var (
	app        *cli.App
	configPath string
	server     *machinery.Server
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

	server, err = machinery.NewServer(conf)
	if err != nil {
		fmt.Println("Could not create server : " + err.Error())
	}

	server.RegisterTask("repo", Repo)

	worker := server.NewWorker("repo", 2)
	err = worker.Launch()
	if err != nil {
		fmt.Println("Could not launch worker : " + err.Error())
	}
		return nil

	}
	app.Run(os.Args)
}
