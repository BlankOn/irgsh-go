package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"

	machinery "github.com/RichardKnop/machinery/v1"
	machineryConfig "github.com/RichardKnop/machinery/v1/config"
	"github.com/blankon/irgsh-go/internal/config"
	"github.com/urfave/cli"
)

var (
	app        *cli.App
	configPath string
	server     *machinery.Server
	version    string

	irgshConfig = config.IrgshConfig{}
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	irgshConfig, err := config.LoadConfig()
	if err != nil {
		log.Fatalln("couldn't load config : ", err)
	}

	_ = exec.Command("bash", "-c", "mkdir -p "+irgshConfig.ISO.Workdir)

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
			Usage:   "initialize iso",
			Action: func(c *cli.Context) error {
				// Do nothing
				return nil
			},
		},
	}

	app.Action = func(c *cli.Context) error {

		go serve()

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

		server.RegisterTask("iso", BuildISO)

		worker := server.NewWorker("iso", 2)
		err = worker.Launch()
		if err != nil {
			fmt.Println("Could not launch worker : " + err.Error())
		}

		return nil

	}
	app.Run(os.Args)
}

func serve() {
	fs := http.FileServer(http.Dir(irgshConfig.ISO.Workdir))
	http.Handle("/", fs)
	log.Println("irgsh-go iso now live on port 8083, serving path : " + irgshConfig.ISO.Workdir)
	log.Fatal(http.ListenAndServe(":8083", nil))
}

func BuildISO(payload string) (next string, err error) {
	fmt.Println("Done")
	return
}
