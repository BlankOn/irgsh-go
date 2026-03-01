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
	"strings"

	"github.com/blankon/irgsh-go/internal/cli/entity"
	"github.com/google/uuid"
)

func (u *CLIUsecase) SubmitPackage(ctx context.Context, params entity.SubmitParams) (entity.SubmitResponse, error) {
	cfg, err := u.Config.Load()
	if err != nil {
		return entity.SubmitResponse{}, ErrConfigMissing
	}

	// Validate chief connectivity (unless ignoring checks)
	if !params.IgnoreChecks {
		versionResp, err := u.Chief.GetVersion(ctx)
		if err != nil {
			return entity.SubmitResponse{}, fmt.Errorf("failed to connect to chief: %w", err)
		}
		if versionResp.Version != u.Version {
			return entity.SubmitResponse{}, fmt.Errorf("client version mismatch: local=%s, chief=%s. Please update your irgsh-cli", u.Version, versionResp.Version)
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
		if _, err := url.ParseRequestURI(params.SourceURL); err != nil {
			return entity.SubmitResponse{}, err
		}
	}
	if params.PackageURL == "" {
		return entity.SubmitResponse{}, errors.New("--package should not be empty")
	}
	if _, err := url.ParseRequestURI(params.PackageURL); err != nil {
		return entity.SubmitResponse{}, err
	}

	// Experimental prompt
	isExperimental := params.IsExperimental
	if !isExperimental {
		confirmed, err := u.Prompter.Confirm("Experimental flag is not set which means the package will be injected to official dev repository. Are you sure you want to continue to submit and build this package?")
		if err != nil {
			return entity.SubmitResponse{}, err
		}
		if !confirmed {
			return entity.SubmitResponse{}, errors.New("submission cancelled by user")
		}
	}

	tmpID := uuid.New().String()
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return entity.SubmitResponse{}, err
	}
	tmpBase := filepath.Join(homeDir, ".irgsh", "tmp")
	tmpDir := filepath.Join(tmpBase, tmpID)

	var downloadableTarballURL string
	if params.SourceURL != "" {
		fmt.Println("sourceUrl: " + params.SourceURL)
		err = u.RepoSync.Sync(params.SourceURL, sourceBranch, filepath.Join(tmpDir, "source"))
		if err != nil {
			fmt.Println(err.Error())
			// Try as downloadable tarball
			downloadableTarballURL = strings.TrimSuffix(params.SourceURL, "\n")
			log.Println("Downloading the tarball " + downloadableTarballURL)
			resp, dlErr := http.Get(downloadableTarballURL)
			if dlErr != nil {
				return entity.SubmitResponse{}, dlErr
			}
			defer resp.Body.Close()

			if mkErr := os.MkdirAll(tmpDir, 0755); mkErr != nil {
				return entity.SubmitResponse{}, mkErr
			}

			tarballName := path.Base(downloadableTarballURL)
			out, createErr := os.Create(filepath.Join(tmpDir, tarballName))
			if createErr != nil {
				return entity.SubmitResponse{}, createErr
			}
			defer out.Close()
			if _, cpErr := out.ReadFrom(resp.Body); cpErr != nil {
				return entity.SubmitResponse{}, cpErr
			}
		}
	}
	fmt.Println("packageUrl: " + params.PackageURL)

	// Clone package repo
	err = u.RepoSync.Sync(params.PackageURL, packageBranch, filepath.Join(tmpDir, "package"))
	if err != nil {
		return entity.SubmitResponse{}, err
	}

	packageDir := filepath.Join(tmpDir, "package")
	controlPath := filepath.Join(packageDir, "debian", "control")
	changelogPath := filepath.Join(packageDir, "debian", "changelog")

	// Extract metadata
	log.Println("Getting package name...")
	packageName, err := u.Debian.ExtractPackageName(controlPath)
	if err != nil {
		return entity.SubmitResponse{}, err
	}
	if packageName == "" {
		return entity.SubmitResponse{}, errors.New("repository does not contain debian spec directory")
	}
	log.Println("Package name: " + packageName)

	log.Println("Getting package version...")
	packageVersion, err := u.Debian.ExtractVersion(changelogPath)
	if err != nil {
		return entity.SubmitResponse{}, err
	}
	log.Println("Package version: " + packageVersion)

	log.Println("Getting package extended version...")
	packageExtendedVersion, err := u.Debian.ExtractExtendedVersion(changelogPath)
	if err != nil {
		return entity.SubmitResponse{}, err
	}
	if packageExtendedVersion == packageVersion {
		packageExtendedVersion = ""
	}
	log.Println("Package extended version: " + packageExtendedVersion)

	log.Println("Getting package last maintainer...")
	packageLastMaintainer, err := u.Debian.ExtractChangelogMaintainer(changelogPath)
	if err != nil {
		return entity.SubmitResponse{}, err
	}
	log.Println(packageLastMaintainer)

	log.Println("Getting uploaders...")
	uploaders, err := u.Debian.ExtractUploaders(controlPath)
	if err != nil {
		return entity.SubmitResponse{}, err
	}

	// Get maintainer identity from GPG key
	log.Println("Getting maintainer identity...")
	maintainerIdentity, err := u.GPG.GetIdentity(cfg.MaintainerSigningKey)
	if err != nil {
		return entity.SubmitResponse{}, err
	}

	// Validate identity matches
	if !params.IgnoreChecks {
		if strings.TrimSpace(uploaders) != strings.TrimSpace(maintainerIdentity) {
			log.Println("The uploader in the debian/control: " + uploaders)
			log.Println("Your signing key identity: " + maintainerIdentity)
			return entity.SubmitResponse{}, errors.New("the uploaders value in the debian/control does not matched with your identity")
		}
		if strings.TrimSpace(packageLastMaintainer) != strings.TrimSpace(maintainerIdentity) {
			log.Println("The last maintainer in the debian/changelog: " + packageLastMaintainer)
			log.Println("Your signing key identity: " + maintainerIdentity)
			return entity.SubmitResponse{}, errors.New("the last maintainer in the debian/changelog does not matched with your identity")
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
			tmpDir, packageName, packageVersion,
			origFileName, packageName, packageVersion,
			packageName, packageVersion,
		)
		if _, shellErr := u.Shell.Output(cmdStr); shellErr != nil {
			log.Printf("error: %v", shellErr)
			log.Println("Failed to create orig tarball.")
		}
	}

	// Rename package dir for debuild
	log.Println("Renaming workdir...")
	renameCmd := fmt.Sprintf("cd %s && mv package %s", tmpDir, packageNameVersion)
	if err := u.Shell.Run(renameCmd); err != nil {
		return entity.SubmitResponse{}, fmt.Errorf("failed to rename workdir: %w", err)
	}

	workDir := filepath.Join(tmpDir, packageNameVersion)

	// dpkg-source --build
	log.Println("Building source package...")
	if err := u.Debian.BuildSource(workDir); err != nil {
		return entity.SubmitResponse{}, fmt.Errorf("dpkg-source failed: %w", err)
	}

	// debsign
	log.Println("Signing the dsc file...")
	if err := u.Debian.Sign(tmpDir, cfg.MaintainerSigningKey); err != nil {
		return entity.SubmitResponse{}, fmt.Errorf("debsign failed: %w", err)
	}

	// dpkg-genbuildinfo
	log.Println("Generating buildinfo file...")
	if err := u.Debian.GenBuildInfo(workDir); err != nil {
		// Some packages need debuild first; try that
		log.Println("Trying debuild before dpkg-genbuildinfo...")
		debuildCmd := fmt.Sprintf("cd %s && debuild -us -uc -b && dpkg-genbuildinfo", workDir)
		if shellErr := u.Shell.RunInteractive(debuildCmd); shellErr != nil {
			log.Printf("debuild + genbuildinfo warning: %v", shellErr)
		}
	}

	// dpkg-genchanges
	log.Println("Generating changes file...")
	genchangesCmd := fmt.Sprintf("cd %s && dpkg-genchanges > ../$(ls .. | grep dsc | tr -d \".dsc\")_source.changes", workDir)
	if err := u.Shell.RunInteractive(genchangesCmd); err != nil {
		return entity.SubmitResponse{}, fmt.Errorf("dpkg-genchanges failed: %w", err)
	}

	// Lintian
	if !params.IgnoreChecks {
		log.Println("Lintian test...")
		lintianCmd := fmt.Sprintf("cd %s && lintian --profile blankon 2>&1", workDir)
		lintianOut, lintianErr := u.Shell.Output(lintianCmd)
		log.Println(lintianOut)
		if lintianErr != nil || strings.Contains(lintianOut, "E:") {
			return entity.SubmitResponse{}, errors.New("failed to pass lintian")
		}
	}

	// Move signed files
	log.Println("Moving generated files to signed dir...")
	moveCmd := fmt.Sprintf("cd %s && mkdir signed && mv *.xz ./signed/ || true && mv *.dsc ./signed/ && mv *.changes ./signed/", tmpDir)
	if err := u.Shell.Run(moveCmd); err != nil {
		return entity.SubmitResponse{}, err
	}

	// Clean up package dir
	log.Println("Cleaning up...")
	if err := u.Shell.Run("rm -rf " + filepath.Join(tmpDir, "package")); err != nil {
		return entity.SubmitResponse{}, err
	}

	// Compress
	log.Println("Compressing...")
	compressCmd := fmt.Sprintf("cd %s && tar -zcvf ../%s.tar.gz .", tmpDir, tmpID)
	if err := u.Shell.Run(compressCmd); err != nil {
		return entity.SubmitResponse{}, err
	}

	// Build submission
	submission := entity.Submission{
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
	jsonByte, _ := json.Marshal(submission)

	// Sign auth token
	log.Println("Signing auth token...")
	tokenContent := b64.StdEncoding.EncodeToString(jsonByte)
	tokenPath := filepath.Join(tmpDir, "token")
	tokenSigPath := filepath.Join(tmpDir, "token.sig")
	if err := os.WriteFile(tokenPath, []byte(tokenContent), 0644); err != nil {
		return entity.SubmitResponse{}, err
	}
	if err := u.GPG.ClearSign(tokenPath, tokenSigPath, cfg.MaintainerSigningKey); err != nil {
		return entity.SubmitResponse{}, fmt.Errorf("failed to sign auth token: %w", err)
	}

	// Upload
	log.Println("Uploading blob...")
	blobPath := filepath.Join(tmpBase, tmpID+".tar.gz")
	uploadResp, err := u.Chief.UploadSubmission(ctx, blobPath, tokenSigPath, func(uploaded, total int64) {
		percentage := float64(uploaded) / float64(total) * 100
		fmt.Printf("\rUploading: %.2f%% (%d/%d bytes)", percentage, uploaded, total)
	})
	if err != nil {
		return entity.SubmitResponse{}, fmt.Errorf("upload failed: %w", err)
	}
	fmt.Println()

	// Submit
	submission.Tarball = uploadResp.ID
	log.Println("Submitting...")
	submitResp, err := u.Chief.SubmitPackage(ctx, submission)
	if err != nil {
		return entity.SubmitResponse{}, err
	}
	if submitResp.Error != "" {
		return entity.SubmitResponse{}, errors.New(submitResp.Error)
	}

	fmt.Println("Submission succeeded. Pipeline ID:")
	fmt.Println(submitResp.PipelineID)

	// Persist pipeline ID
	if err := u.Pipelines.SavePackageID(submitResp.PipelineID); err != nil {
		log.Printf("warning: failed to save pipeline ID: %v", err)
	}

	return submitResp, nil
}

func (u *CLIUsecase) PackageStatus(ctx context.Context, pipelineID string) (entity.PackageStatus, error) {
	cfg, err := u.Config.Load()
	if err != nil {
		return entity.PackageStatus{}, ErrConfigMissing
	}
	_ = cfg

	if pipelineID == "" {
		pipelineID, err = u.Pipelines.LoadPackageID()
		if err != nil || pipelineID == "" {
			return entity.PackageStatus{}, ErrPipelineIDMissing
		}
	}

	fmt.Println("Checking the status of " + pipelineID + " ...")
	return u.Chief.GetPackageStatus(ctx, pipelineID)
}

func (u *CLIUsecase) PackageLog(ctx context.Context, pipelineID string) (buildLog, repoLog string, err error) {
	cfg, loadErr := u.Config.Load()
	if loadErr != nil {
		return "", "", ErrConfigMissing
	}
	_ = cfg

	if pipelineID == "" {
		pipelineID, err = u.Pipelines.LoadPackageID()
		if err != nil || pipelineID == "" {
			return "", "", ErrPipelineIDMissing
		}
	}

	fmt.Println("Fetching the logs of " + pipelineID + " ...")

	// Check if pipeline is finished
	status, err := u.Chief.GetPackageStatus(ctx, pipelineID)
	if err != nil {
		return "", "", err
	}
	if status.State == "STARTED" {
		return "", "", errors.New("the pipeline is not finished yet")
	}

	buildLog, err = u.Chief.FetchLog(ctx, pipelineID+".build.log")
	if err != nil {
		return "", "", err
	}
	if strings.Contains(buildLog, "404 page not found") {
		return "", "", errors.New("builder log is not found. The worker/pipeline may terminated ungracefully")
	}

	repoLog, err = u.Chief.FetchLog(ctx, pipelineID+".repo.log")
	if err != nil {
		return "", "", err
	}
	if strings.Contains(repoLog, "404 page not found") {
		return "", "", errors.New("repo log is not found. The worker/pipeline may terminated ungracefully")
	}

	return buildLog, repoLog, nil
}
