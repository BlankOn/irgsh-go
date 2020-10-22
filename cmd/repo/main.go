package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	machinery "github.com/RichardKnop/machinery/v1"
	machineryConfig "github.com/RichardKnop/machinery/v1/config"
	"github.com/urfave/cli"

	"github.com/blankon/irgsh-go/internal/config"
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

		server.RegisterTask("repo", Repo)
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
