package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	machinery "github.com/RichardKnop/machinery/v1"
	machineryConfig "github.com/RichardKnop/machinery/v1/config"
	"github.com/urfave/cli"

	"github.com/blankon/irgsh-go/internal/config"
	"github.com/blankon/irgsh-go/internal/monitoring"

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
	monitoringRegistry   *monitoring.Registry
)

type Submission struct {
	TaskUUID               string    `json:"taskUUID"`
	Timestamp              time.Time `json:"timestamp"`
	PackageName            string    `json:"packageName"`
	PackageVersion         string    `json:"packageVersion"`
	PackageExtendedVersion string    `json:"packageExtendedVersion"`
	PackageURL             string    `json:"packageUrl"`
	SourceURL              string    `json:"sourceUrl"`
	Maintainer             string    `json:"maintainer"`
	MaintainerFingerprint  string    `json:"maintainerFingerprint"`
	Component              string    `json:"component"`
	IsExperimental         bool      `json:"isExperimental"`
	ForceVersion           bool      `json:"forceVersion"`
	Tarball                string    `json:"tarball"`
	PackageBranch          string    `json:"packageBranch"`
	SourceBranch           string    `json:"sourceBranch"`
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

	// Initialize monitoring registry if enabled
	if irgshConfig.Monitoring.Enabled {
		ttl := time.Duration(irgshConfig.Monitoring.InstanceTimeout) * time.Second
		monitoringRegistry, err = monitoring.NewRegistry(irgshConfig.Redis, ttl)
		if err != nil {
			log.Printf("Failed to initialize monitoring registry: %v\n", err)
			log.Println("Continuing without monitoring...")
			irgshConfig.Monitoring.Enabled = false
		} else {
			log.Println("Monitoring registry initialized successfully")
		}
	}

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
	// APIs
	http.HandleFunc("/api/v1/artifacts", artifactHTTPEndpoint.GetArtifactListHandler)
	http.HandleFunc("/api/v1/submit", PackageSubmitHandler)
	http.HandleFunc("/api/v1/status", BuildStatusHandler)
	http.HandleFunc("/api/v1/retry", RetryHandler)
	http.HandleFunc("/api/v1/artifact-upload", artifactUploadHandler())
	http.HandleFunc("/api/v1/log-upload", logUploadHandler())
	http.HandleFunc("/api/v1/submission-upload", submissionUploadHandler())
	http.HandleFunc("/api/v1/build-iso", BuildISOHandler)
	http.HandleFunc("/api/v1/version", VersionHandler)

	// Pages
	http.HandleFunc("/maintainers", MaintainersHandler)

	// Index handler (catch-all, must be registered last)
	http.HandleFunc("/", indexHandler)
	// Static file routes
	artifactFs := http.FileServer(http.Dir(irgshConfig.Chief.Workdir + "/artifacts"))
	http.Handle("/artifacts/", http.StripPrefix("/artifacts/", artifactFs))
	logFs := http.FileServer(http.Dir(irgshConfig.Chief.Workdir + "/logs"))
	http.Handle("/logs/", http.StripPrefix("/logs/", logFs))
	submissionFs := http.FileServer(http.Dir(irgshConfig.Chief.Workdir + "/submissions"))
	http.Handle("/submissions/", http.StripPrefix("/submissions/", submissionFs))

	// Start monitoring cleanup job if enabled
	if irgshConfig.Monitoring.Enabled && monitoringRegistry != nil {
		go startInstanceCleanup()
	}

	port := os.Getenv("PORT")
	if len(port) < 1 {
		port = "8080"
	}
	log.Println("irgsh-go chief now live on port " + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// startInstanceCleanup runs a background job to cleanup stale instances
func startInstanceCleanup() {
	interval := time.Duration(irgshConfig.Monitoring.CleanupInterval) * time.Second
	timeout := time.Duration(irgshConfig.Monitoring.InstanceTimeout) * time.Second

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("Instance cleanup job started (interval: %v, timeout: %v)\n", interval, timeout)

	for range ticker.C {
		if err := monitoringRegistry.CleanupStaleInstances(timeout); err != nil {
			log.Printf("Failed to cleanup stale instances: %v\n", err)
		}
	}
}

func Move(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	in.Close()
	out.Close()

	return os.Remove(src)
}
