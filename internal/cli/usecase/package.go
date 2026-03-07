package usecase

import (
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/blankon/irgsh-go/internal/cli/domain"

	"github.com/google/uuid"
)

// safeDebianName matches safe Debian package names and version strings.
// Rejects shell metacharacters while allowing all valid Debian identifiers.
var safeDebianName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.+~:-]*$`)

// sq shell-quotes a string by wrapping it in single quotes with proper escaping.
func sq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func (u *CLIUsecase) SubmitPackage(ctx context.Context, params domain.SubmitParams) (domain.SubmitResponse, error) {
	cfg, err := u.config.Load()
	if err != nil {
		return domain.SubmitResponse{}, fmt.Errorf("%w: %w", ErrConfigMissing, err)
	}

	// Validate chief connectivity (unless ignoring checks)
	if !params.IgnoreChecks {
		versionResp, err := u.chief.GetVersion(ctx)
		if err != nil {
			return domain.SubmitResponse{}, fmt.Errorf("failed to connect to chief: %w", err)
		}
		if versionResp.Version != u.version {
			return domain.SubmitResponse{}, fmt.Errorf("client version mismatch: local=%s, chief=%s. Please update your irgsh-cli", u.version, versionResp.Version)
		}
	}

	// Defaults
	component := params.Component
	if component == "" {
		component = "main"
	}
	packageBranch := params.PackageBranch
	if packageBranch == "" {
		packageBranch = "master"
	}
	sourceBranch := params.SourceBranch
	if sourceBranch == "" {
		sourceBranch = "master"
	}

	// Validate URLs
	if params.SourceURL != "" {
		srcURL, err := url.Parse(params.SourceURL)
		if err != nil || srcURL.Host == "" || (srcURL.Scheme != "http" && srcURL.Scheme != "https") {
			return domain.SubmitResponse{}, errors.New("--source must be a valid http or https URL")
		}
	}
	if params.PackageURL == "" {
		return domain.SubmitResponse{}, errors.New("--package should not be empty")
	}
	if pkgURL, err := url.Parse(params.PackageURL); err != nil || pkgURL.Host == "" || (pkgURL.Scheme != "http" && pkgURL.Scheme != "https") {
		return domain.SubmitResponse{}, errors.New("--package must be a valid http or https URL")
	}

	// Experimental prompt
	isExperimental := params.IsExperimental
	if !isExperimental {
		confirmed, err := u.prompter.Confirm("Experimental flag is not set which means the package will be injected to official dev repository. Are you sure you want to continue to submit and build this package?")
		if err != nil {
			return domain.SubmitResponse{}, err
		}
		if !confirmed {
			return domain.SubmitResponse{}, errors.New("submission cancelled by user")
		}
	}

	tmpID := uuid.New().String()
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return domain.SubmitResponse{}, err
	}
	tmpBase := filepath.Join(homeDir, ".irgsh", "tmp")
	tmpDir := filepath.Join(tmpBase, tmpID)
	defer os.RemoveAll(tmpDir)
	defer os.Remove(filepath.Join(tmpBase, tmpID+".tar.gz"))

	var downloadableTarballURL string
	if params.SourceURL != "" {
		fmt.Println("sourceUrl: " + params.SourceURL)
		err = u.repoSync.Sync(params.SourceURL, sourceBranch, filepath.Join(tmpDir, "source"))
		if err != nil {
			// Only fall back to tarball download if the repo/branch was not found.
			// Other errors (e.g. network, permission) should propagate immediately.
			if !errors.Is(err, domain.ErrRepoOrBranchNotFound) {
				return domain.SubmitResponse{}, err
			}
			log.Println(err)
			// Try as downloadable tarball
			downloadableTarballURL = strings.TrimSuffix(params.SourceURL, "\n")
			log.Println("Downloading the tarball " + downloadableTarballURL)
			dlReq, dlErr := http.NewRequestWithContext(ctx, http.MethodGet, downloadableTarballURL, nil)
			if dlErr != nil {
				return domain.SubmitResponse{}, dlErr
			}
			dlClient := &http.Client{Timeout: 5 * time.Minute}
			resp, dlErr := dlClient.Do(dlReq)
			if dlErr != nil {
				return domain.SubmitResponse{}, dlErr
			}
			defer resp.Body.Close()

			if mkErr := os.MkdirAll(tmpDir, 0755); mkErr != nil {
				return domain.SubmitResponse{}, mkErr
			}

			tarballName := path.Base(downloadableTarballURL)
			out, createErr := os.Create(filepath.Join(tmpDir, tarballName))
			if createErr != nil {
				return domain.SubmitResponse{}, createErr
			}
			defer out.Close()
			if _, cpErr := out.ReadFrom(resp.Body); cpErr != nil {
				return domain.SubmitResponse{}, cpErr
			}
		}
	}
	fmt.Println("packageUrl: " + params.PackageURL)

	// Clone package repo
	err = u.repoSync.Sync(params.PackageURL, packageBranch, filepath.Join(tmpDir, "package"))
	if err != nil {
		return domain.SubmitResponse{}, err
	}

	packageDir := filepath.Join(tmpDir, "package")
	controlPath := filepath.Join(packageDir, "debian", "control")
	changelogPath := filepath.Join(packageDir, "debian", "changelog")

	// Extract metadata
	log.Println("Getting package name...")
	packageName, err := u.debian.ExtractPackageName(controlPath)
	if err != nil {
		return domain.SubmitResponse{}, err
	}
	if packageName == "" {
		return domain.SubmitResponse{}, errors.New("repository does not contain debian spec directory")
	}
	if !safeDebianName.MatchString(packageName) {
		return domain.SubmitResponse{}, fmt.Errorf("invalid package name %q: contains unsafe characters", packageName)
	}
	log.Println("Package name: " + packageName)

	log.Println("Getting package version...")
	packageVersion, err := u.debian.ExtractVersion(changelogPath)
	if err != nil {
		return domain.SubmitResponse{}, err
	}
	if !safeDebianName.MatchString(packageVersion) {
		return domain.SubmitResponse{}, fmt.Errorf("invalid package version %q: contains unsafe characters", packageVersion)
	}
	log.Println("Package version: " + packageVersion)

	log.Println("Getting package extended version...")
	packageExtendedVersion, err := u.debian.ExtractExtendedVersion(changelogPath)
	if err != nil {
		return domain.SubmitResponse{}, err
	}
	if packageExtendedVersion == packageVersion {
		packageExtendedVersion = ""
	}
	if packageExtendedVersion != "" && !safeDebianName.MatchString(packageExtendedVersion) {
		return domain.SubmitResponse{}, fmt.Errorf("invalid package extended version %q: contains unsafe characters", packageExtendedVersion)
	}
	log.Println("Package extended version: " + packageExtendedVersion)

	log.Println("Getting package last maintainer...")
	packageLastMaintainer, err := u.debian.ExtractChangelogMaintainer(changelogPath)
	if err != nil {
		return domain.SubmitResponse{}, err
	}
	log.Println(packageLastMaintainer)

	log.Println("Getting uploaders...")
	uploaders, err := u.debian.ExtractUploaders(controlPath)
	if err != nil {
		return domain.SubmitResponse{}, err
	}

	// Get maintainer identity from GPG key
	log.Println("Getting maintainer identity...")
	maintainerIdentity, err := u.gpg.GetIdentity(cfg.MaintainerSigningKey)
	if err != nil {
		return domain.SubmitResponse{}, err
	}

	// Validate identity matches
	if !params.IgnoreChecks {
		if strings.TrimSpace(uploaders) != strings.TrimSpace(maintainerIdentity) {
			log.Println("The uploader in the debian/control: " + uploaders)
			log.Println("Your signing key identity: " + maintainerIdentity)
			return domain.SubmitResponse{}, errors.New("the uploaders value in the debian/control does not matched with your identity")
		}
		if strings.TrimSpace(packageLastMaintainer) != strings.TrimSpace(maintainerIdentity) {
			log.Println("The last maintainer in the debian/changelog: " + packageLastMaintainer)
			log.Println("Your signing key identity: " + maintainerIdentity)
			return domain.SubmitResponse{}, errors.New("the last maintainer in the debian/changelog does not matched with your identity")
		}
	}

	// Determine package name with version
	packageNameVersion := packageName + "-" + packageVersion
	if packageExtendedVersion != "" {
		packageNameVersion += "-" + packageExtendedVersion
	}

	// Create orig tarball from source if provided (not a downloadable tarball)
	if params.SourceURL != "" && downloadableTarballURL == "" {
		origFileName := packageName + "_" + strings.Split(packageVersion, "-")[0]
		log.Println("Creating orig tarball...")
		cmdStr := fmt.Sprintf(
			"cd %s && mkdir -p tmp && mv source tmp && cd tmp && mv source %s-%s && tar cfJ %s.orig.tar.xz %s-%s && rm -rf %s-%s && mv *.xz .. && cd .. && rm -rf tmp",
			sq(tmpDir), packageName, packageVersion,
			origFileName, packageName, packageVersion,
			packageName, packageVersion,
		)
		if _, shellErr := u.shell.Output(cmdStr); shellErr != nil {
			return domain.SubmitResponse{}, fmt.Errorf("failed to create orig tarball: %w", shellErr)
		}
	}

	// Rename package dir for debuild
	log.Println("Renaming workdir...")
	renameCmd := fmt.Sprintf("cd %s && mv package %s", sq(tmpDir), packageNameVersion)
	if err := u.shell.Run(renameCmd); err != nil {
		return domain.SubmitResponse{}, fmt.Errorf("failed to rename workdir: %w", err)
	}

	workDir := filepath.Join(tmpDir, packageNameVersion)

	// dpkg-source --build
	log.Println("Building source package...")
	if err := u.debian.BuildSource(workDir); err != nil {
		return domain.SubmitResponse{}, fmt.Errorf("dpkg-source failed: %w", err)
	}

	// debsign
	log.Println("Signing the dsc file...")
	if err := u.debian.Sign(tmpDir, cfg.MaintainerSigningKey); err != nil {
		return domain.SubmitResponse{}, fmt.Errorf("debsign failed: %w", err)
	}

	// dpkg-genbuildinfo
	log.Println("Generating buildinfo file...")
	if err := u.debian.GenBuildInfo(workDir); err != nil {
		// Some packages need debuild first; try that
		log.Println("Trying debuild before dpkg-genbuildinfo...")
		debuildCmd := fmt.Sprintf("cd %s && debuild -us -uc -b && dpkg-genbuildinfo", sq(workDir))
		if shellErr := u.shell.RunInteractive(debuildCmd); shellErr != nil {
			return domain.SubmitResponse{}, fmt.Errorf("dpkg-genbuildinfo failed (debuild fallback also failed: %w): %w", shellErr, err)
		}
	}

	// dpkg-genchanges
	log.Println("Generating changes file...")
	dscMatches, err := filepath.Glob(filepath.Join(tmpDir, "*.dsc"))
	if err != nil {
		return domain.SubmitResponse{}, fmt.Errorf("failed to find .dsc file: %w", err)
	}
	if len(dscMatches) == 0 {
		return domain.SubmitResponse{}, errors.New("no .dsc file found after dpkg-source")
	}
	dscBase := strings.TrimSuffix(filepath.Base(dscMatches[0]), ".dsc")
	genchangesCmd := fmt.Sprintf("cd %s && dpkg-genchanges > %s", sq(workDir), sq(filepath.Join(tmpDir, dscBase+"_source.changes")))
	if err := u.shell.RunInteractive(genchangesCmd); err != nil {
		return domain.SubmitResponse{}, fmt.Errorf("dpkg-genchanges failed: %w", err)
	}

	// Lintian
	if !params.IgnoreChecks {
		log.Println("Lintian test...")
		lintianCmd := fmt.Sprintf("cd %s && lintian --profile blankon 2>&1", sq(workDir))
		lintianOut, lintianErr := u.shell.Output(lintianCmd)
		log.Println(lintianOut)
		if lintianErr != nil || strings.Contains(lintianOut, "E:") {
			return domain.SubmitResponse{}, errors.New("failed to pass lintian")
		}
	}

	// Move signed files
	log.Println("Moving generated files to signed dir...")
	moveCmd := fmt.Sprintf("cd %s && mkdir signed && mv *.dsc ./signed/ && mv *.changes ./signed/", sq(tmpDir))
	if err := u.shell.Run(moveCmd); err != nil {
		return domain.SubmitResponse{}, err
	}
	// .xz files only exist for source-built packages; ignore if absent
	_ = u.shell.Run(fmt.Sprintf("cd %s && mv *.xz ./signed/", sq(tmpDir)))

	// Clean up package dir
	log.Println("Cleaning up...")
	if err := u.shell.Run("rm -rf " + sq(filepath.Join(tmpDir, "package"))); err != nil {
		return domain.SubmitResponse{}, err
	}

	// Compress
	log.Println("Compressing...")
	compressCmd := fmt.Sprintf("cd %s && tar -zcvf ../%s.tar.gz .", sq(tmpDir), tmpID)
	if err := u.shell.Run(compressCmd); err != nil {
		return domain.SubmitResponse{}, err
	}

	// Build submission
	submission := domain.Submission{
		PackageName:            packageName,
		PackageVersion:         packageVersion,
		PackageExtendedVersion: packageExtendedVersion,
		PackageURL:             params.PackageURL,
		SourceURL:              params.SourceURL,
		Maintainer:             maintainerIdentity,
		MaintainerFingerprint:  cfg.MaintainerSigningKey,
		Component:              component,
		IsExperimental:         isExperimental,
		ForceVersion:           params.ForceVersion,
		PackageBranch:          packageBranch,
		SourceBranch:           sourceBranch,
	}
	jsonByte, err := json.Marshal(submission)
	if err != nil {
		return domain.SubmitResponse{}, fmt.Errorf("failed to marshal submission: %w", err)
	}

	// Sign auth token
	log.Println("Signing auth token...")
	tokenContent := b64.StdEncoding.EncodeToString(jsonByte)
	tokenPath := filepath.Join(tmpDir, "token")
	tokenSigPath := filepath.Join(tmpDir, "token.sig")
	if err := os.WriteFile(tokenPath, []byte(tokenContent), 0600); err != nil {
		return domain.SubmitResponse{}, err
	}
	if err := u.gpg.ClearSign(tokenPath, tokenSigPath, cfg.MaintainerSigningKey); err != nil {
		return domain.SubmitResponse{}, fmt.Errorf("failed to sign auth token: %w", err)
	}

	// Upload
	log.Println("Uploading blob...")
	blobPath := filepath.Join(tmpBase, tmpID+".tar.gz")
	uploadResp, err := u.chief.UploadSubmission(ctx, blobPath, tokenSigPath, func(uploaded, total int64) {
		if total > 0 {
			percentage := float64(uploaded) / float64(total) * 100
			fmt.Printf("\rUploading: %.2f%% (%d/%d bytes)", percentage, uploaded, total)
		}
	})
	if err != nil {
		return domain.SubmitResponse{}, fmt.Errorf("upload failed: %w", err)
	}
	fmt.Println()

	// Submit
	submission.Tarball = uploadResp.ID
	log.Println("Submitting...")
	submitResp, err := u.chief.SubmitPackage(ctx, submission)
	if err != nil {
		return domain.SubmitResponse{}, err
	}
	if submitResp.Error != "" {
		return domain.SubmitResponse{}, errors.New(submitResp.Error)
	}

	fmt.Println("Submission succeeded. Pipeline ID:")
	fmt.Println(submitResp.PipelineID)

	// Persist pipeline ID
	if err := u.pipelines.SavePackageID(submitResp.PipelineID); err != nil {
		log.Printf("warning: failed to save pipeline ID: %v", err)
	}

	return submitResp, nil
}

func (u *CLIUsecase) PackageStatus(ctx context.Context, pipelineID string) (domain.PackageStatus, error) {
	if _, err := u.config.Load(); err != nil {
		return domain.PackageStatus{}, fmt.Errorf("%w: %w", ErrConfigMissing, err)
	}

	if pipelineID == "" {
		var err error
		pipelineID, err = u.pipelines.LoadPackageID()
		if err != nil || pipelineID == "" {
			return domain.PackageStatus{}, ErrPipelineIDMissing
		}
	}

	fmt.Println("Checking the status of " + pipelineID + " ...")
	return u.chief.GetPackageStatus(ctx, pipelineID)
}

func (u *CLIUsecase) PackageLog(ctx context.Context, pipelineID string) (buildLog, repoLog string, err error) {
	if _, loadErr := u.config.Load(); loadErr != nil {
		return "", "", fmt.Errorf("%w: %w", ErrConfigMissing, loadErr)
	}

	if pipelineID == "" {
		pipelineID, err = u.pipelines.LoadPackageID()
		if err != nil || pipelineID == "" {
			return "", "", ErrPipelineIDMissing
		}
	}

	fmt.Println("Fetching the logs of " + pipelineID + " ...")

	// Check if pipeline is finished
	status, err := u.chief.GetPackageStatus(ctx, pipelineID)
	if err != nil {
		return "", "", err
	}
	if status.State == "STARTED" {
		return "", "", errors.New("the pipeline is not finished yet")
	}

	buildLog, err = u.chief.FetchLog(ctx, pipelineID+".build.log")
	if err != nil {
		if isHTTPNotFound(err) {
			return "", "", errors.New("builder log is not found. The worker/pipeline may have terminated ungracefully")
		}
		return "", "", err
	}

	repoLog, err = u.chief.FetchLog(ctx, pipelineID+".repo.log")
	if err != nil {
		if isHTTPNotFound(err) {
			return "", "", errors.New("repo log is not found. The worker/pipeline may have terminated ungracefully")
		}
		return "", "", err
	}

	return buildLog, repoLog, nil
}
