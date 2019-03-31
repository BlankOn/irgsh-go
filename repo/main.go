package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	machinery "github.com/RichardKnop/machinery/v1"
	"github.com/RichardKnop/machinery/v1/config"
	"github.com/hpcloud/tail"
	"github.com/urfave/cli"
	validator "gopkg.in/go-playground/validator.v9"
)

type Repository struct {
	// Distribution repository related values
	DistName                   string `validate:"required"` // BlankOn
	DistLabel                  string `validate:"required"` // BlankOn
	DistCodename               string `validate:"required"` // verbeek
	DistComponents             string `validate:"required"` // main restricted extras extras-restricted
	DistSupportedArchitectures string `validate:"required"` // amd64 source
	DistVersion                string `validate:"required"` // 12.0
	DistVersionDesc            string `validate:"required"` // BlankOn Linux 12.0 Verbeek
	DistSigningKey             string `validate:"required"` // 55BD65A0B3DA3A59ACA60932E2FE388D53B56A71
	UpstreamName               string `validate:"required"` // merge.sid
	UpstreamDistCodename       string `validate:"required"` // sid
	UpstreamDistUrl            string `validate:"required"` // http://kartolo.sby.datautama.net.id/debian
	UpstreamDistComponents     string `validate:"required"` // main non-free>restricted contrib>extras
}

var (
	app          *cli.App
	configPath   string
	server       *machinery.Server
	workdir      string
	chiefAddress string
	repository   Repository
)

func loadConfig() (*config.Config, error) {
	if configPath != "" {
		return config.NewFromYaml(configPath, true)
	}
	return config.NewFromEnvironment(true)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	workdir = os.Getenv("IRGSH_REPO_WORKDIR")
	if len(workdir) == 0 {
		workdir = "/tmp/irgsh/repo"
	}

	chiefAddress = os.Getenv("IRGSH_CHIEF_ADDRESS")
	if len(chiefAddress) == 0 {
		log.Fatal("No IRGSH Chief address provided.")
		os.Exit(1)
	}

	// IRGSH related config from ENV
	repository.DistName = os.Getenv("IRGSH_REPO_DIST_NAME")
	repository.DistLabel = os.Getenv("IRGSH_REPO_DIST_LABEL")
	repository.DistCodename = os.Getenv("IRGSH_REPO_DIST_CODENAME")
	repository.DistComponents = os.Getenv("IRGSH_REPO_DIST_COMPONENTS")
	repository.DistSupportedArchitectures = os.Getenv("IRGSH_REPO_DIST_SUPPORTED_ARCHITECTURES")
	repository.DistVersion = os.Getenv("IRGSH_REPO_DIST_VERSION")
	repository.DistVersionDesc = os.Getenv("IRGSH_REPO_DIST_VERSION_DESC")
	repository.DistSigningKey = os.Getenv("IRGSH_REPO_DIST_SIGNING_KEY")
	repository.UpstreamName = os.Getenv("IRGSH_REPO_UPSTREAM_NAME")
	repository.UpstreamDistCodename = os.Getenv("IRGSH_REPO_UPSTREAM_DIST_CODENAME")
	repository.UpstreamDistUrl = os.Getenv("IRGSH_REPO_UPSTREAM_DIST_URL")
	repository.UpstreamDistComponents = os.Getenv("IRGSH_REPO_UPSTREAM_DIST_COMPONENTS")

	validate := validator.New()
	err := validate.Struct(repository)
	if err != nil {
		log.Fatal(err.Error())
		os.Exit(1)
	}

	app = cli.NewApp()
	app.Name = "irgsh-go"
	app.Usage = "irgsh-go distributed packager"
	app.Author = "BlankOn Developer"
	app.Email = "blankon-dev@googlegroups.com"
	app.Version = "IRGSH_GO_VERSION"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "c",
			Value:       "",
			Destination: &configPath,
			Usage:       "Path to a configuration file",
		},
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

		conf, err := loadConfig()
		if err != nil {
			fmt.Println("Failed to load : " + err.Error())
		}

		go serve()

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

func serve() {
	fs := http.FileServer(http.Dir(workdir + "/" + repository.DistCodename + "/www"))
	http.Handle("/", fs)
	log.Println("irgsh-go chief now live on port 8082")
	log.Fatal(http.ListenAndServe(":8082", nil))
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
