package main

import (
	"context"
	"fmt"

	"github.com/blankon/irgsh-go/internal/cli/entity"
	"github.com/blankon/irgsh-go/internal/cli/usecase"
	"github.com/urfave/cli"
)

func buildApp(svc *usecase.CLIUsecase, version string) *cli.App {
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
			Action: packageSubmitAction(svc),
			Subcommands: []cli.Command{
				{
					Name:   "status",
					Usage:  "Check status of a package build pipeline",
					Action: packageStatusAction(svc),
				},
				{
					Name:   "log",
					Usage:  "Read the logs of a package build pipeline",
					Action: packageLogAction(svc),
				},
			},
		},
		{
			Name:   "retry",
			Usage:  "Retry a failed pipeline",
			Action: retryAction(svc),
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
					Action: livebuildSubmitAction(svc),
				},
				{
					Name:   "status",
					Usage:  "Check status of an ISO build pipeline",
					Action: livebuildStatusAction(svc),
				},
				{
					Name:   "log",
					Usage:  "Read the logs of an ISO build pipeline",
					Action: livebuildLogAction(svc),
				},
			},
		},
		{
			Name:   "update",
			Usage:  "Update the irgsh-cli tool",
			Action: updateAction(svc),
		},
	}

	return app
}

func configAction(svc *usecase.CLIUsecase) cli.ActionFunc {
	return func(c *cli.Context) error {
		cfg := entity.Config{
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

func packageSubmitAction(svc *usecase.CLIUsecase) cli.ActionFunc {
	return func(c *cli.Context) error {
		params := entity.SubmitParams{
			PackageURL:     c.String("package"),
			SourceURL:      c.String("source"),
			Component:      c.String("component"),
			PackageBranch:  c.String("package-branch"),
			SourceBranch:   c.String("source-branch"),
			IsExperimental: c.Bool("experimental"),
			IgnoreChecks:   c.Bool("ignore-checks"),
			ForceVersion:   c.Bool("force-version"),
		}
		_, err := svc.SubmitPackage(context.Background(), params)
		return err
	}
}

func packageStatusAction(svc *usecase.CLIUsecase) cli.ActionFunc {
	return func(c *cli.Context) error {
		pipelineID := c.Args().First()
		status, err := svc.PackageStatus(context.Background(), pipelineID)
		if err != nil {
			return err
		}
		fmt.Printf("Job Status:   %s\n", status.JobStatus)
		fmt.Printf("Build Status: %s\n", status.BuildStatus)
		fmt.Printf("Repo Status:  %s\n", status.RepoStatus)
		return nil
	}
}

func packageLogAction(svc *usecase.CLIUsecase) cli.ActionFunc {
	return func(c *cli.Context) error {
		pipelineID := c.Args().First()
		buildLog, repoLog, err := svc.PackageLog(context.Background(), pipelineID)
		if err != nil {
			return err
		}
		fmt.Println(buildLog)
		fmt.Println(repoLog)
		return nil
	}
}

func livebuildSubmitAction(svc *usecase.CLIUsecase) cli.ActionFunc {
	return func(c *cli.Context) error {
		_, err := svc.SubmitISO(context.Background(), c.String("lb-url"), c.String("lb-branch"))
		return err
	}
}

func livebuildStatusAction(svc *usecase.CLIUsecase) cli.ActionFunc {
	return func(c *cli.Context) error {
		pipelineID := c.Args().First()
		status, err := svc.ISOStatus(context.Background(), pipelineID)
		if err != nil {
			return err
		}
		fmt.Printf("Job Status: %s\n", status.JobStatus)
		fmt.Printf("ISO Status: %s\n", status.ISOStatus)
		return nil
	}
}

func livebuildLogAction(svc *usecase.CLIUsecase) cli.ActionFunc {
	return func(c *cli.Context) error {
		pipelineID := c.Args().First()
		logResult, err := svc.ISOLog(context.Background(), pipelineID)
		if err != nil {
			return err
		}
		fmt.Println(logResult)
		return nil
	}
}

func retryAction(svc *usecase.CLIUsecase) cli.ActionFunc {
	return func(c *cli.Context) error {
		pipelineID := c.Args().First()
		_, err := svc.RetryPipeline(context.Background(), pipelineID)
		return err
	}
}

func updateAction(svc *usecase.CLIUsecase) cli.ActionFunc {
	return func(c *cli.Context) error {
		return svc.UpdateCLI(context.Background())
	}
}
