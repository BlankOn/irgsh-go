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

	artifactEndpoint "github.com/blankon/irgsh-go/internal/artifact/endpoint"
	artifactRepo "github.com/blankon/irgsh-go/internal/artifact/repo"
	artifactService "github.com/blankon/irgsh-go/internal/artifact/service"
	chiefrepository "github.com/blankon/irgsh-go/internal/chief/repository"
	chiefusecase "github.com/blankon/irgsh-go/internal/chief/usecase"
	"github.com/blankon/irgsh-go/internal/config"
	"github.com/blankon/irgsh-go/internal/monitoring"
)

var (
	app     *cli.App
	server  *machinery.Server
	version string

	irgshConfig config.IrgshConfig

	artifactHTTPEndpoint *artifactEndpoint.ArtifactHTTPEndpoint
	monitoringRegistry   *monitoring.Registry

	chiefService *chiefusecase.ChiefUsecase
	chiefStorage *chiefrepository.Storage
	chiefGPG     *chiefrepository.GPG
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	var err error
	irgshConfig, err = config.LoadConfig()
	if err != nil {
		log.Fatalln(err)
	}

	err = os.MkdirAll(irgshConfig.Chief.Workdir, 0755)
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(irgshConfig.Chief.Workdir)

	artifactHTTPEndpoint = artifactEndpoint.NewArtifactHTTPEndpoint(
		artifactService.NewArtifactService(
			artifactRepo.NewFileRepo(irgshConfig.Chief.Workdir)))

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

	chiefStorage = chiefrepository.NewStorage(irgshConfig.Chief.Workdir)
	chiefGPG = chiefrepository.NewGPG(irgshConfig.Chief.GnupgDir, irgshConfig.IsDev)

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

		chiefService = chiefusecase.NewChiefUsecase(
			irgshConfig,
			server,
			monitoringRegistry,
			chiefStorage,
			chiefGPG,
			version,
		)

		serve()

		return nil
	}
	app.Run(os.Args)
}

func serve() {
	http.HandleFunc("/api/v1/artifacts", artifactHTTPEndpoint.GetArtifactListHandler)
	http.HandleFunc("/api/v1/submit", PackageSubmitHandler)
	http.HandleFunc("/api/v1/status", BuildStatusHandler)
	http.HandleFunc("/api/v1/retry", RetryHandler)
	http.HandleFunc("/api/v1/artifact-upload", artifactUploadHandler())
	http.HandleFunc("/api/v1/log-upload", logUploadHandler())
	http.HandleFunc("/api/v1/submission-upload", submissionUploadHandler())
	http.HandleFunc("/api/v1/build-iso", BuildISOHandler)
	http.HandleFunc("/api/v1/version", VersionHandler)

	http.HandleFunc("/maintainers", MaintainersHandler)

	http.HandleFunc("/", indexHandler)

	artifactFs := http.FileServer(http.Dir(irgshConfig.Chief.Workdir + "/artifacts"))
	http.Handle("/artifacts/", http.StripPrefix("/artifacts/", artifactFs))
	logFs := http.FileServer(http.Dir(irgshConfig.Chief.Workdir + "/logs"))
	http.Handle("/logs/", http.StripPrefix("/logs/", logFs))
	submissionFs := http.FileServer(http.Dir(irgshConfig.Chief.Workdir + "/submissions"))
	http.Handle("/submissions/", http.StripPrefix("/submissions/", submissionFs))

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
