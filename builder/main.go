package main

import (
	"fmt"
	"log"
	"os"

	machinery "github.com/RichardKnop/machinery/v1"
	"github.com/RichardKnop/machinery/v1/config"
	"github.com/urfave/cli"
	"github.com/hpcloud/tail"
)

var (
	app        *cli.App
	configPath string
	server     *machinery.Server
	workdir    string
	signingKey string
	isBuild    string
)

func loadConfig() (*config.Config, error) {
	if configPath != "" {
		return config.NewFromYaml(configPath, true)
	}
	return config.NewFromEnvironment(true)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

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
		cli.StringFlag{
			Name:  "build",
			Value: "true",
			Usage: "Path to a configuration file",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:    "init",
			Aliases: []string{"i"},
			Usage:   "initialize builder",
			Action: func(c *cli.Context) error {
				err := InitBase()
				return err
			},
		},
		{
			Name:    "update",
			Aliases: []string{"i"},
			Usage:   "update base.tgz",
			Action: func(c *cli.Context) error {
				err := UpdateBase()
				return err
			},
		},
	}

	app.Action = func(c *cli.Context) error {

		conf, err := loadConfig()
		if err != nil {
			fmt.Println("Failed to load : " + err.Error())
		}

		signingKey = os.Getenv("IRGSH_BUILDER_SIGNING_KEY")
		if len(signingKey) == 0 {
			log.Fatal("No signing key provided.")
			os.Exit(1)
		}

		// IRGSH related config from ENV
		workdir = os.Getenv("IRGSH_BUILDER_WORKDIR")
		if len(workdir) == 0 {
			workdir = "/tmp"
		}

		server, err = machinery.NewServer(conf)
		if err != nil {
			fmt.Println("Could not create server : " + err.Error())
		}

		server.RegisterTask("build", Build)

		worker := server.NewWorker("builder", 2)
		err = worker.Launch()
		if err != nil {
			fmt.Println("Could not launch worker : " + err.Error())
		}
		return nil

	}
	app.Run(os.Args)
}

func StreamLog(path string) {
	t, err := tail.TailFile(path, tail.Config{Follow: true})
	if err != nil {
		log.Printf("error: %v\n", err)
	}
	for line := range t.Lines {
		fmt.Println(line.Text)
	}
}
