package main

import (
	"log"
	"os"
	"os/user"
	"path/filepath"

	"github.com/blankon/irgsh-go/internal/cli/repository"
	"github.com/blankon/irgsh-go/internal/cli/usecase"
)

var version string

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	basePath := filepath.Join(usr.HomeDir, ".irgsh")

	// Build repositories
	shell := &repository.ShellRunner{}
	configStore := repository.NewFileConfigStore(basePath)
	pipelineStore := repository.NewFilePipelineStore(basePath)
	chiefClient := repository.NewHTTPChiefClient(configStore)
	repoSync := repository.NewGitRepoSync(filepath.Join(basePath, "cache"))
	debianPkg := repository.NewShellDebianPackager(shell)
	gpgSigner := repository.NewShellGPGSigner(shell)
	releases := repository.NewGitHubReleaseFetcher()
	updater := &repository.GoUpdateApplier{}
	prompter := &repository.TerminalPrompter{}

	// Build usecase
	svc := usecase.NewCLIUsecase(
		configStore, pipelineStore, chiefClient, shell,
		repoSync, debianPkg, gpgSigner, releases, updater, prompter, version,
	)

	// Build CLI app with handlers
	app := buildApp(svc, version)
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
