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
	if params.SourceDist == "" {
		return domain.SubmitResponse{}, errors.New("--source-dist is required")
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
	fmt.Printf("Target distribution: %s\n", params.Dist)
	fmt.Printf("Source: %s (%s/%s)\n", params.SourceURL, params.SourceDist, sourceComponent)
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
		SourceDist:      params.SourceDist,
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

	// Resolve before anything is queued: the maintainer can be told what the
	// import really costs while it is still cheap to say no.
	if !params.SkipCheck {
		names, checkErr := u.planImport(ctx, params, sourceComponent)
		switch {
		case checkErr == nil:
			submission.PackageNames = names
		case errors.Is(checkErr, errImportDeclined):
			return domain.SubmitResponse{}, checkErr
		case errors.Is(checkErr, errCheckUnavailable), errors.Is(checkErr, errNoTarget):
			fmt.Printf("Skipping the dependency check: %v\n", checkErr)
			fmt.Println("The repo worker will still check before injecting.")
		default:
			var depErr *ImportDependencyError
			if errors.As(checkErr, &depErr) && !params.IgnoreDependencies {
				return domain.SubmitResponse{}, fmt.Errorf("%w\n\n"+
					"Import it anyway with --ignore-dependencies, or check without submitting with --dry-run",
					depErr)
			}
			if errors.As(checkErr, &depErr) {
				fmt.Println("Warning: the packages are not installable, importing anyway (--ignore-dependencies)")
			} else {
				fmt.Printf("Skipping the dependency check: %v\n", checkErr)
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

// errImportDeclined reports that the maintainer said no to the extra packages
// an import would pull in.
var errImportDeclined = errors.New("import cancelled")

// planImport resolves what has to be imported and, when that is more than was
// asked for, puts the bill in front of the maintainer before anything is
// queued. It returns the package list to submit.
func (u *CLIUsecase) planImport(ctx context.Context, params domain.ImportParams, sourceComponent string) ([]string, error) {
	// Ask chief where these packages are going, rather than assuming this
	// machine is configured with the same repository.
	info, infoErr := u.chief.GetRepoInfo(ctx, params.Dist)
	if infoErr != nil {
		return nil, fmt.Errorf("%w: chief could not be asked which repository this targets (%v)",
			errNoTarget, infoErr)
	}
	targets, targetDesc, err := targetSources(info)
	if err != nil {
		return nil, err
	}

	checkParams := ImportCheckParams{
		SourceURL:       params.SourceURL,
		SourceDist:      params.SourceDist,
		SourceComponent: sourceComponent,
		PackageNames:    params.PackageNames,
		TargetSources:   targets,
	}

	fmt.Println("Checking the packages against " + targetDesc + " ...")

	sandbox, err := u.newImportSandbox(checkParams)
	if err != nil {
		return nil, err
	}
	defer sandbox.close()

	targetView, err := u.newTargetView(checkParams)
	if err != nil {
		return nil, err
	}
	defer targetView.close()

	plan, err := u.resolveImport(sandbox, targetView, params.PackageNames)
	if err != nil {
		return nil, err
	}

	if len(plan.Blockers) > 0 {
		return nil, &ImportDependencyError{Output: renderBlockers(plan.Blockers) + "\n" + plan.Output}
	}
	if plan.Output != "" {
		return nil, &ImportDependencyError{Output: plan.Output}
	}

	pulled := plan.Set.pulled()
	if len(pulled) == 0 {
		fmt.Println("Dependency check passed.")
		return params.PackageNames, nil
	}

	fmt.Print(renderPulledReport(params.PackageNames, pulled))

	// The extra packages are the maintainer's call: they are what turns
	// "import strongswan" into a much larger change to the repository.
	switch {
	case params.AssumeYes:
		fmt.Println("Importing them as well (--yes)")
	case u.prompter == nil:
		return nil, fmt.Errorf("%w: there is no way to ask about the extra packages here, "+
			"re-run with --yes to accept them", errImportDeclined)
	default:
		confirmed, confirmErr := u.prompter.Confirm(
			fmt.Sprintf("Import all of them into %s/%s?", params.Dist, params.Component))
		if confirmErr != nil || !confirmed {
			return nil, fmt.Errorf("%w: the extra packages were not accepted", errImportDeclined)
		}
	}

	// Everything the resolver added has to be named in the submission, or the
	// repo worker would import only what was asked for and inject a set that
	// does not resolve.
	return plan.Set.representatives(params.PackageNames), nil
}
