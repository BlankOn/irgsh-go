package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"

	machinery "github.com/RichardKnop/machinery/v1"
	machineryConfig "github.com/RichardKnop/machinery/v1/config"
	"github.com/ghodss/yaml"
	"github.com/hpcloud/tail"
	"github.com/urfave/cli"
	validator "gopkg.in/go-playground/validator.v9"
)

var (
	app        *cli.App
	configPath string
	server     *machinery.Server

	irgshConfig IrgshConfig
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load config
	configPath = os.Getenv("IRGSH_CONFIG_PATH")
	if len(configPath) == 0 {
		configPath = "/etc/irgsh/config.yml"
	}
	irgshConfig = IrgshConfig{}
	yamlFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	err = yaml.Unmarshal(yamlFile, &irgshConfig)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	validate := validator.New()
	err = validate.Struct(irgshConfig.Repo)
	if err != nil {
		log.Fatal(err.Error())
		os.Exit(1)
	}
	_ = exec.Command("bash", "-c", "mkdir -p "+irgshConfig.Repo.Workdir)

	app = cli.NewApp()
	app.Name = "irgsh-go"
	app.Usage = "irgsh-go distributed packager"
	app.Author = "BlankOn Developer"
	app.Email = "blankon-dev@googlegroups.com"
	app.Version = "IRGSH_GO_VERSION"

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

		worker := server.NewWorker("repo", 1)
		err = worker.Launch()
		if err != nil {
			fmt.Println("Could not launch worker : " + err.Error())
		}
		return nil

	}
	app.Run(os.Args)
}

func serve() {
	fs := http.FileServer(http.Dir(irgshConfig.Repo.Workdir + "/" + irgshConfig.Repo.DistCodename + "/www"))
	http.Handle("/", fs)
	log.Println("irgsh-go chief now live on port 8082")
	log.Fatal(http.ListenAndServe(":8082", nil))
}

func StreamLog(path string) {
	_ = exec.Command("bash", "-c", "touch "+path).Run()
	t, err := tail.TailFile(path, tail.Config{Follow: true})
	if err != nil {
		log.Printf("error: %v\n", err)
	}
	for line := range t.Lines {
		fmt.Println(line.Text)
	}
}
