package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
	"github.com/blankon/irgsh-go/internal/storage"
)

var (
	version string
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	app := cli.NewApp()
	app.Name = "irgsh-go"
	app.Usage = "irgsh-go distributed packager"
	app.Author = "BlankOn Developer"
	app.Email = "blankon-dev@googlegroups.com"
	app.Version = version

	app.Action = func(c *cli.Context) error {
		irgshConfig, err := config.LoadConfig()
		if err != nil {
			log.Fatalln(err)
		}

		if err := os.MkdirAll(irgshConfig.Chief.Workdir, 0755); err != nil {
			log.Fatalln(err)
		}
		log.Println(irgshConfig.Chief.Workdir)

		artifactHTTPEndpoint := artifactEndpoint.NewArtifactHTTPEndpoint(
			artifactService.NewArtifactService(
				artifactRepo.NewFileRepo(irgshConfig.Chief.Workdir)))

		// Initialize SQLite storage for job persistence
		storageDB, err := storage.NewDB(irgshConfig.Storage.DatabasePath)
		if err != nil {
			log.Fatalf("Failed to initialize storage database: %v\n", err)
		}
		log.Printf("Storage database initialized at %s\n", irgshConfig.Storage.DatabasePath)

		// Initialize monitoring registry if enabled
		var monitoringRegistry *monitoring.Registry
		if irgshConfig.Monitoring.Enabled {
			ttl := time.Duration(irgshConfig.Monitoring.InstanceTimeout) * time.Second
			monitoringRegistry, err = monitoring.NewRegistry(
				irgshConfig.Redis,
				ttl,
				storageDB,
				irgshConfig.Storage.MaxJobs,
				irgshConfig.Storage.MaxISOJobs,
			)
			if err != nil {
				log.Printf("Failed to initialize monitoring registry: %v\n", err)
				log.Println("Continuing without monitoring...")
				irgshConfig.Monitoring.Enabled = false
			} else {
				log.Println("Monitoring registry initialized successfully")
			}
		}

		chiefStorage := chiefrepository.NewStorage(irgshConfig.Chief.Workdir)
		chiefGPG := chiefrepository.NewGPG(irgshConfig.Chief.GnupgDir, irgshConfig.IsDev)

		server, err := machinery.NewServer(
			&machineryConfig.Config{
				Broker:        irgshConfig.Redis,
				ResultBackend: irgshConfig.Redis,
				DefaultQueue:  "irgsh",
			},
		)
		if err != nil {
			log.Fatalf("Could not create server: %v", err)
		}

		taskQueue := chiefrepository.NewMachineryTaskQueue(server)
		svc, err := chiefusecase.NewChiefUsecase(
			irgshConfig,
			taskQueue,
			monitoringRegistry,
			chiefStorage,
			chiefGPG,
			version,
		)
		if err != nil {
			log.Fatalf("Failed to initialize chief service: %v\n", err)
		}
		chiefService = svc

		httpServer := setupRoutes(irgshConfig, artifactHTTPEndpoint)

		if irgshConfig.Monitoring.Enabled && monitoringRegistry != nil {
			go startInstanceCleanup(irgshConfig, monitoringRegistry)
		}

		// Graceful shutdown
		shutdownDone := make(chan struct{})
		go func() {
			handleShutdown(httpServer, storageDB, monitoringRegistry)
			close(shutdownDone)
		}()

		port := os.Getenv("PORT")
		if len(port) < 1 {
			port = "8080"
		}
		httpServer.Addr = ":" + port
		log.Println("irgsh-go chief now live on port " + port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}

		<-shutdownDone
		return nil
	}
	app.Run(os.Args)
}

var chiefService ChiefService

func setupRoutes(cfg config.IrgshConfig, artifactEP *artifactEndpoint.ArtifactHTTPEndpoint) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/artifacts", artifactEP.GetArtifactListHandler)
	mux.HandleFunc("/api/v1/submit", PackageSubmitHandler)
	mux.HandleFunc("/api/v1/status", BuildStatusHandler)
	mux.HandleFunc("/api/v1/retry", RetryHandler)
	mux.HandleFunc("/api/v1/artifact-upload", artifactUploadHandler())
	mux.HandleFunc("/api/v1/log-upload", logUploadHandler())
	mux.HandleFunc("/api/v1/submission-upload", submissionUploadHandler())
	mux.HandleFunc("/api/v1/build-iso", BuildISOHandler)
	mux.HandleFunc("/api/v1/iso-status", ISOStatusHandler)
	mux.HandleFunc("/api/v1/version", VersionHandler)

	mux.HandleFunc("/maintainers", MaintainersHandler)

	mux.HandleFunc("/", indexHandler)

	artifactFs := http.FileServer(http.Dir(cfg.Chief.Workdir + "/artifacts"))
	mux.Handle("/artifacts/", http.StripPrefix("/artifacts/", artifactFs))
	logFs := http.FileServer(http.Dir(cfg.Chief.Workdir + "/logs"))
	mux.Handle("/logs/", http.StripPrefix("/logs/", logFs))
	submissionFs := http.FileServer(http.Dir(cfg.Chief.Workdir + "/submissions"))
	mux.Handle("/submissions/", http.StripPrefix("/submissions/", submissionFs))

	return &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
}

func startInstanceCleanup(cfg config.IrgshConfig, registry *monitoring.Registry) {
	interval := time.Duration(cfg.Monitoring.CleanupInterval) * time.Second
	timeout := time.Duration(cfg.Monitoring.InstanceTimeout) * time.Second

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("Instance cleanup job started (interval: %v, timeout: %v)\n", interval, timeout)

	for range ticker.C {
		if err := registry.CleanupStaleInstances(timeout); err != nil {
			log.Printf("Failed to cleanup stale instances: %v\n", err)
		}
	}
}

func handleShutdown(httpServer *http.Server, storageDB *storage.DB, registry *monitoring.Registry) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v\n", err)
	} else {
		log.Println("HTTP server stopped")
	}

	if storageDB != nil {
		if err := storageDB.Close(); err != nil {
			log.Printf("Error closing storage database: %v\n", err)
		} else {
			log.Println("Storage database closed")
		}
	}

	if registry != nil {
		if err := registry.Close(); err != nil {
			log.Printf("Error closing monitoring registry: %v\n", err)
		} else {
			log.Println("Monitoring registry closed")
		}
	}
}
