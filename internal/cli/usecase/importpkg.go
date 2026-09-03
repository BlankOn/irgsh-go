package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/blankon/irgsh-go/internal/cli/domain"
)

// safeDebianBinaryName matches Debian binary package names.
var safeDebianBinaryName = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]+$`)

// SplitPackageNames splits a --package-name value into individual package
// names. Both comma and whitespace separated lists are accepted, so
// "grub-pc,calamares" and "grub-pc calamares" mean the same thing.
func SplitPackageNames(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})

	var names []string
	for _, field := range fields {
		if name := strings.TrimSpace(field); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// SubmitImport asks chief to import already built packages from an external
// Debian repository into ours.
func (u *CLIUsecase) SubmitImport(ctx context.Context, params domain.ImportParams) (domain.SubmitResponse, error) {
	cfg, err := u.config.Load()
	if err != nil {
		return domain.SubmitResponse{}, fmt.Errorf("%w: %w", ErrConfigMissing, err)
	}

	if params.SourceURL == "" {
		return domain.SubmitResponse{}, errors.New("--source is required")
	}
	if params.Dist == "" {
		return domain.SubmitResponse{}, errors.New("--dist is required")
	}
	if params.TargetDist == "" {
		return domain.SubmitResponse{}, errors.New("--repo-dist is required")
	}
	if len(params.PackageNames) == 0 {
		return domain.SubmitResponse{}, errors.New("--package-name is required")
	}
	for _, name := range params.PackageNames {
		if !safeDebianBinaryName.MatchString(name) {
			return domain.SubmitResponse{}, fmt.Errorf("invalid package name: %q", name)
		}
	}

	component := params.Component
	if component == "" {
		component = "main"
	}
	sourceComponent := params.SourceComponent
	if sourceComponent == "" {
		sourceComponent = "main"
	}

	// Record who triggered the import. There is nothing to verify here, but
	// the dashboard should still show who asked for it.
	maintainer, err := u.gpg.GetIdentity(cfg.MaintainerSigningKey)
	if err != nil {
		return domain.SubmitResponse{}, fmt.Errorf("failed to read the identity of your signing key: %w", err)
	}

	fmt.Println("Submitting package import job...")
	fmt.Printf("Target distribution: %s\n", params.TargetDist)
	fmt.Printf("Source: %s (%s/%s)\n", params.SourceURL, params.Dist, sourceComponent)
	fmt.Printf("Packages: %s\n", strings.Join(params.PackageNames, ", "))
	fmt.Printf("Target component: %s\n", component)
	fmt.Printf("Importer: %s\n", maintainer)
	if params.Insecure {
		fmt.Println("Warning: --insecure skips verification of the source repository's signature")
	}
	if params.DryRun {
		fmt.Println("Dry run: the packages will be fetched and checked, but not injected")
	}
	if params.IgnoreDependencies {
		fmt.Println("Warning: --ignore-dependencies imports even if the packages are not installable")
	}

	submission := domain.ImportSubmission{
		SourceURL:       params.SourceURL,
		Dist:            params.Dist,
		TargetDist:      params.TargetDist,
		SourceComponent: sourceComponent,
		PackageNames:    params.PackageNames,
		Component:       component,
		IsExperimental:  params.IsExperimental,
		ForceVersion:    params.ForceVersion,
		Insecure:        params.Insecure,
		KeyringPath:     params.KeyringPath,
		Maintainer:      maintainer,
		DryRun:          params.DryRun,

		IgnoreDependencies: params.IgnoreDependencies,
	}

	// Check here first: the maintainer's machine runs the distribution these
	// packages are going into, so it can answer before anything is queued.
	if !params.SkipCheck {
		// Ask chief where these packages are going, rather than assuming this
		// machine is configured with the same repository.
		var targets []string
		var targetDesc string
		info, infoErr := u.chief.GetRepoInfo(ctx, params.TargetDist)
		if infoErr != nil {
			fmt.Printf("Could not ask chief which repository this targets (%v), falling back to this machine's sources\n", infoErr)
		}
		targets, targetDesc, infoErr = targetSources(info)
		if infoErr != nil {
			fmt.Printf("Skipping the local dependency check: %v\n", infoErr)
			targets = nil
		}

		if len(targets) > 0 {
			fmt.Println("Checking the packages against " + targetDesc + " ...")
		}
		switch checkErr := u.checkImportLocally(ImportCheckParams{
			SourceURL:       params.SourceURL,
			Dist:            params.Dist,
			SourceComponent: sourceComponent,
			PackageNames:    params.PackageNames,
			TargetSources:   targets,
		}); {
		case checkErr == nil:
			fmt.Println("Dependency check passed.")
		case errors.Is(checkErr, errCheckUnavailable), errors.Is(checkErr, errNoSystemSources), errors.Is(checkErr, errNoTarget):
			fmt.Printf("Skipping the local dependency check: %v\n", checkErr)
			fmt.Println("The repo worker will still check before injecting.")
		default:
			var depErr *ImportDependencyError
			if errors.As(checkErr, &depErr) && !params.IgnoreDependencies {
				return domain.SubmitResponse{}, fmt.Errorf("%s: %w\n\n"+
					"Import it anyway with --ignore-dependencies, or check without submitting with --dry-run",
					targetDesc, depErr)
			}
			if errors.As(checkErr, &depErr) {
				fmt.Println("Warning: the packages are not installable here, importing anyway (--ignore-dependencies)")
			} else {
				fmt.Printf("Skipping the local dependency check: %v\n", checkErr)
			}
		}
	}

	resp, err := u.chief.SubmitImport(ctx, submission)
	if err != nil {
		return domain.SubmitResponse{}, err
	}
	if resp.Error != "" {
		return domain.SubmitResponse{}, errors.New(resp.Error)
	}

	fmt.Println("Import submitted successfully!")
	fmt.Println("Pipeline ID: " + resp.PipelineID)

	if err := u.pipelines.SaveImportID(resp.PipelineID); err != nil {
		fmt.Printf("warning: failed to save pipeline ID: %v\n", err)
	}

	return resp, nil
}

func (u *CLIUsecase) ImportStatus(ctx context.Context, pipelineID string) (domain.ImportStatus, error) {
	if _, err := u.config.Load(); err != nil {
		return domain.ImportStatus{}, fmt.Errorf("%w: %w", ErrConfigMissing, err)
	}

	var err error
	if pipelineID == "" {
		pipelineID, err = u.pipelines.LoadImportID()
		if err != nil || pipelineID == "" {
			return domain.ImportStatus{}, ErrPipelineIDMissing
		}
	}

	fmt.Println("Checking the status of " + pipelineID + " ...")
	return u.chief.GetImportStatus(ctx, pipelineID)
}

func (u *CLIUsecase) ImportLog(ctx context.Context, pipelineID string) (string, error) {
	if _, err := u.config.Load(); err != nil {
		return "", fmt.Errorf("%w: %w", ErrConfigMissing, err)
	}

	var err error
	if pipelineID == "" {
		pipelineID, err = u.pipelines.LoadImportID()
		if err != nil || pipelineID == "" {
			return "", ErrPipelineIDMissing
		}
	}

	fmt.Println("Fetching the logs of " + pipelineID + " ...")

	logResult, err := u.chief.FetchLog(ctx, pipelineID+".import.log")
	if err != nil {
		if isHTTPNotFound(err) {
			return "", errors.New("import log is not found. The worker/pipeline may have terminated ungracefully")
		}
		return "", err
	}

	return logResult, nil
}
