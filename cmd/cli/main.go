package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/blankon/irgsh-go/internal/cli/repository"
	"github.com/blankon/irgsh-go/internal/cli/usecase"
)

var version string

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	if err := runCLI(sigCtx, os.Args, usr.HomeDir, version); err != nil {
		log.Fatal(err)
	}
}

func profilePath(home, target string) string {
	return filepath.Join(home, ".irgsh", "targets", target)
}

func selectedTarget(args []string) (string, error) {
	target := "dev"
	if len(args) > 1 {
		switch {
		case args[1] == "--target":
			if len(args) < 3 {
				return "", fmt.Errorf("--target requires dev or prod")
			}
			target = args[2]
		case strings.HasPrefix(args[1], "--target="):
			target = strings.TrimPrefix(args[1], "--target=")
		}
	}
	if target != "dev" && target != "prod" {
		return "", fmt.Errorf("invalid target %q: choose dev or prod", target)
	}
	return target, nil
}

func runCLI(ctx context.Context, args []string, home, version string) error {
	target, err := selectedTarget(args)
	if err != nil {
		return err
	}
	basePath := profilePath(home, target)

	// Build repositories
	shell := &repository.ShellRunner{}
	configStore := repository.NewFileConfigStore(basePath)
	pipelineStore := repository.NewFilePipelineStore(basePath)
	chiefClient := repository.NewHTTPChiefClient(configStore)
	repoSync := repository.NewGitRepoSync(filepath.Join(basePath, "cache"))
	debianPkg := repository.NewShellDebianPackager(shell)
	gpgSigner := repository.NewShellGPGSigner()
	releases := repository.NewGitHubReleaseFetcher()
	updater := &repository.GoUpdateApplier{}
	prompter := &repository.TerminalPrompter{}

	// Build usecase
	svc := usecase.NewCLIUsecase(
		configStore, pipelineStore, chiefClient, shell,
		repoSync, debianPkg, gpgSigner, releases, updater, prompter, version,
	)

	// Build CLI app with handlers
	app := buildApp(ctx, svc, version, target)
	return app.Run(args)
}
