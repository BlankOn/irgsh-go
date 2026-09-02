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
	if _, err := u.config.Load(); err != nil {
		return domain.SubmitResponse{}, fmt.Errorf("%w: %w", ErrConfigMissing, err)
	}

	if params.SourceURL == "" {
		return domain.SubmitResponse{}, errors.New("--source is required")
	}
	if params.Dist == "" {
		return domain.SubmitResponse{}, errors.New("--dist is required")
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

	fmt.Println("Submitting package import job...")
	fmt.Printf("Source: %s (%s/%s)\n", params.SourceURL, params.Dist, sourceComponent)
	fmt.Printf("Packages: %s\n", strings.Join(params.PackageNames, ", "))
	fmt.Printf("Target component: %s\n", component)
	if params.Insecure {
		fmt.Println("Warning: --insecure skips verification of the source repository's signature")
	}

	submission := domain.ImportSubmission{
		SourceURL:       params.SourceURL,
		Dist:            params.Dist,
		SourceComponent: sourceComponent,
		PackageNames:    params.PackageNames,
		Component:       component,
		IsExperimental:  params.IsExperimental,
		ForceVersion:    params.ForceVersion,
		Insecure:        params.Insecure,
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
