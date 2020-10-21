package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	machinery "github.com/RichardKnop/machinery/v1"
	machineryConfig "github.com/RichardKnop/machinery/v1/config"
	"github.com/urfave/cli"

	"github.com/blankon/irgsh-go/internal/config"

	artifactEndpoint "github.com/blankon/irgsh-go/internal/artifact/endpoint"
	artifactRepo "github.com/blankon/irgsh-go/internal/artifact/repo"
	artifactService "github.com/blankon/irgsh-go/internal/artifact/service"
)

var (
	app     *cli.App
	server  *machinery.Server
	version string

	irgshConfig config.IrgshConfig

	artifactHTTPEndpoint *artifactEndpoint.ArtifactHTTPEndpoint
)

type Submission struct {
	TaskUUID       string    `json:"taskUUID"`
	Timestamp      time.Time `json:"timestamp"`
	SourceURL      string    `json:"sourceUrl"`
	PackageURL     string    `json:"packageUrl"`
	Tarball        string    `json:"tarball"`
	IsExperimental bool      `json:"isExperimental"`
}

type ArtifactsPayloadResponse struct {
	Data []string `json:"data"`
}

type SubmitPayloadResponse struct {
	PipelineId string   `json:"pipelineId"`
	Jobs       []string `json:"jobs,omitempty"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	var err error
	irgshConfig, err = config.LoadConfig()
	if err != nil {
		log.Fatalln(err)
	}

	// Prepare workdir
	err = os.MkdirAll(irgshConfig.Chief.Workdir, 0755)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(irgshConfig.Chief.Workdir)

	artifactHTTPEndpoint = artifactEndpoint.NewArtifactHTTPEndpoint(
		artifactService.NewArtifactService(
			artifactRepo.NewFileRepo(irgshConfig.Chief.Workdir)))

	app = cli.NewApp()
	app.Name = "irgsh-go"
	app.Usage = "irgsh-go distributed packager"
	app.Author = "BlankOn Developer"
	app.Email = "blankon-dev@googlegroups.com"
	app.Version = version

	app.Action = func(c *cli.Context) error {
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

		serve()

		return nil
	}
	app.Run(os.Args)

}

func serve() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/api/v1/artifacts", artifactHTTPEndpoint.GetArtifactListHandler)
	http.HandleFunc("/api/v1/submit", PackageSubmitHandler)
	http.HandleFunc("/api/v1/status", BuildStatusHandler)
	http.HandleFunc("/api/v1/artifact-upload", artifactUploadHandler())
	http.HandleFunc("/api/v1/log-upload", logUploadHandler())
	http.HandleFunc("/api/v1/build-iso", BuildISOHandler)

	artifactFs := http.FileServer(http.Dir(irgshConfig.Chief.Workdir + "/artifacts"))
	http.Handle("/artifacts/", http.StripPrefix("/artifacts/", artifactFs))

	logFs := http.FileServer(http.Dir(irgshConfig.Chief.Workdir + "/logs"))
	http.Handle("/logs/", http.StripPrefix("/logs/", logFs))

	port := os.Getenv("PORT")
	if len(port) < 1 {
		port = "8080"
	}
	log.Println("irgsh-go chief now live on port " + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "irgsh-chief "+app.Version)
}
