package main

import (
	"context"
	"fmt"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/urfave/cli"
)

// CLIService defines the operations available to CLI command handlers.
type CLIService interface {
	SaveConfig(cfg domain.Config) error
	SubmitPackage(ctx context.Context, params domain.SubmitParams) (domain.SubmitResponse, error)
	PackageStatus(ctx context.Context, pipelineID string) (domain.PackageStatus, error)
	PackageLog(ctx context.Context, pipelineID string) (buildLog, repoLog string, err error)
	SubmitISO(ctx context.Context, repoURL, branch string) (domain.SubmitResponse, error)
	ISOStatus(ctx context.Context, pipelineID string) (domain.ISOStatus, error)
	ISOLog(ctx context.Context, pipelineID string) (string, error)
	RetryPipeline(ctx context.Context, pipelineID string) (domain.RetryResponse, error)
	UpdateCLI(ctx context.Context) error
}

func buildApp(ctx context.Context, svc CLIService, version string) *cli.App {
	app := cli.NewApp()
	app.Name = "irgsh-go"
	app.Usage = "irgsh-go distributed packager"
	app.Author = "BlankOn Developer"
	app.Email = "blankon-dev@googlegroups.com"
	app.Version = version

	app.Commands = []cli.Command{
		{
			Name:  "config",
			Usage: "Configure irgsh-cli",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "chief",
					Usage: "Chief address",
				},
				cli.StringFlag{
					Name:  "key",
					Usage: "Maintainer signing key",
				},
			},
			Action: configAction(svc),
		},
		{
			Name:  "package",
			Usage: "Submit a package build job, or use subcommands (status, log)",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "source",
					Usage: "Source URL",
				},
				cli.StringFlag{
					Name:  "package",
					Usage: "Package URL",
				},
				cli.StringFlag{
					Name:  "component",
					Usage: "Repository component",
				},
				cli.StringFlag{
					Name:  "package-branch",
					Usage: "Package git branch",
				},
				cli.StringFlag{
					Name:  "source-branch",
					Usage: "Source git branch",
				},
				cli.BoolFlag{
					Name:  "experimental",
					Usage: "Enable experimental flag",
				},
				cli.BoolFlag{
					Name:  "ignore-checks",
					Usage: "Ignore all validation check and restriction",
				},
				cli.BoolFlag{
					Name:  "force-version",
					Usage: "Force overwrite existing package version in repository",
				},
			},
			Action: packageSubmitAction(ctx, svc),
			Subcommands: []cli.Command{
				{
					Name:   "status",
					Usage:  "Check status of a package build pipeline",
					Action: packageStatusAction(ctx, svc),
				},
				{
					Name:   "log",
					Usage:  "Read the logs of a package build pipeline",
					Action: packageLogAction(ctx, svc),
				},
			},
		},
		{
			Name:   "retry",
			Usage:  "Retry a failed pipeline",
			Action: retryAction(ctx, svc),
		},
		{
			Name:  "livebuild",
			Usage: "ISO build commands (submit, status, log)",
			Subcommands: []cli.Command{
				{
					Name:  "submit",
					Usage: "Submit an ISO build job",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "lb-url",
							Usage: "Live build git repository URL (required)",
						},
						cli.StringFlag{
							Name:  "lb-branch",
							Usage: "Live build git branch name (required)",
						},
					},
					Action: livebuildSubmitAction(ctx, svc),
				},
				{
					Name:   "status",
					Usage:  "Check status of an ISO build pipeline",
					Action: livebuildStatusAction(ctx, svc),
				},
				{
					Name:   "log",
					Usage:  "Read the logs of an ISO build pipeline",
					Action: livebuildLogAction(ctx, svc),
				},
			},
		},
		{
			Name:   "update",
			Usage:  "Update the irgsh-cli tool",
			Action: updateAction(ctx, svc),
		},
	}

	return app
}

func configAction(svc CLIService) cli.ActionFunc {
	return func(c *cli.Context) error {
		cfg := domain.Config{
			ChiefAddress:         c.String("chief"),
			MaintainerSigningKey: c.String("key"),
		}
		if err := svc.SaveConfig(cfg); err != nil {
			return err
		}
		fmt.Println("irgsh-cli is successfully configured. Happy hacking!")
		return nil
	}
}

func packageSubmitAction(ctx context.Context, svc CLIService) cli.ActionFunc {
	return func(c *cli.Context) error {
		params := domain.SubmitParams{
			PackageURL:     c.String("package"),
			SourceURL:      c.String("source"),
			Component:      c.String("component"),
			PackageBranch:  c.String("package-branch"),
			SourceBranch:   c.String("source-branch"),
			IsExperimental: c.Bool("experimental"),
			IgnoreChecks:   c.Bool("ignore-checks"),
			ForceVersion:   c.Bool("force-version"),
		}
		_, err := svc.SubmitPackage(ctx, params)
		return err
	}
}

func packageStatusAction(ctx context.Context, svc CLIService) cli.ActionFunc {
	return func(c *cli.Context) error {
		pipelineID := c.Args().First()
		status, err := svc.PackageStatus(ctx, pipelineID)
		if err != nil {
			return err
		}
		fmt.Printf("Job Status:   %s\n", status.JobStatus)
		fmt.Printf("Build Status: %s\n", status.BuildStatus)
		fmt.Printf("Repo Status:  %s\n", status.RepoStatus)
		return nil
	}
}

func packageLogAction(ctx context.Context, svc CLIService) cli.ActionFunc {
	return func(c *cli.Context) error {
		pipelineID := c.Args().First()
		buildLog, repoLog, err := svc.PackageLog(ctx, pipelineID)
		if err != nil {
			return err
		}
		fmt.Println(buildLog)
		fmt.Println(repoLog)
		return nil
	}
}

func livebuildSubmitAction(ctx context.Context, svc CLIService) cli.ActionFunc {
	return func(c *cli.Context) error {
		_, err := svc.SubmitISO(ctx, c.String("lb-url"), c.String("lb-branch"))
		return err
	}
}

func livebuildStatusAction(ctx context.Context, svc CLIService) cli.ActionFunc {
	return func(c *cli.Context) error {
		pipelineID := c.Args().First()
		status, err := svc.ISOStatus(ctx, pipelineID)
		if err != nil {
			return err
		}
		fmt.Printf("Job Status: %s\n", status.JobStatus)
		fmt.Printf("ISO Status: %s\n", status.ISOStatus)
		return nil
	}
}

func livebuildLogAction(ctx context.Context, svc CLIService) cli.ActionFunc {
	return func(c *cli.Context) error {
		pipelineID := c.Args().First()
		logResult, err := svc.ISOLog(ctx, pipelineID)
		if err != nil {
			return err
		}
		fmt.Println(logResult)
		return nil
	}
}

func retryAction(ctx context.Context, svc CLIService) cli.ActionFunc {
	return func(c *cli.Context) error {
		pipelineID := c.Args().First()
		_, err := svc.RetryPipeline(ctx, pipelineID)
		return err
	}
}

func updateAction(ctx context.Context, svc CLIService) cli.ActionFunc {
	return func(c *cli.Context) error {
		return svc.UpdateCLI(ctx)
	}
}
