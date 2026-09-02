package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/blankon/irgsh-go/internal/notification"
	"github.com/blankon/irgsh-go/pkg/systemutil"
	"github.com/manifoldco/promptui"
)

func uploadLog(logPath string, id string) {
	// Upload the log to chief
	if info, err := os.Stat(logPath); err != nil {
		fmt.Printf("error: log file %s is not uploadable: %v\n", logPath, err)
		return
	} else {
		fmt.Printf("Uploading log file %s (%d bytes) to %s\n", logPath, info.Size(), irgshConfig.Chief.Address)
	}
	cmdStr := "curl -v -F 'uploadFile=@" + logPath + "' '" + irgshConfig.Chief.Address + "/api/v1/log-upload?id=" + id + "&type=repo'"
	fmt.Println(cmdStr)
	_, err := systemutil.CmdExec(
		cmdStr,
		"Uploading log file to chief",
		"",
	)
	if err != nil {
		fmt.Printf("error: failed to upload log file %s: %v\n", logPath, err)
	}
}

// describeArtifactDir reports what is actually present on disk for a task, so a
// failing download or injection step leaves a trace of what the repo worker saw.
func describeArtifactDir(artifactDir string, taskUUID string) string {
	target := artifactDir + "/" + taskUUID
	var b strings.Builder
	b.WriteString("##### Artifact directory listing: " + target + "\n")

	entries, err := os.ReadDir(target)
	if err != nil {
		b.WriteString("  unable to read " + target + ": " + err.Error() + "\n")
		tarball := target + ".tar.gz"
		if info, statErr := os.Stat(tarball); statErr == nil {
			b.WriteString(fmt.Sprintf("  tarball %s exists (%d bytes)\n", tarball, info.Size()))
		} else {
			b.WriteString("  tarball " + tarball + " is missing: " + statErr.Error() + "\n")
		}
		return b.String()
	}
	if len(entries) == 0 {
		b.WriteString("  (empty)\n")
		return b.String()
	}
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			b.WriteString("  " + entry.Name() + " (unable to stat: " + infoErr.Error() + ")\n")
			continue
		}
		b.WriteString(fmt.Sprintf("  %s\t%d bytes\n", entry.Name(), info.Size()))
	}
	return b.String()
}

func sendRepoNotification(taskUUID, status string, jobInfo notification.JobNotificationInfo) {
	notification.SendJobNotification(
		irgshConfig.Notification.WebhookURL,
		"Repo",
		taskUUID,
		status,
		jobInfo,
	)
}

// Main task wrapper
func Repo(payload string) (err error) {
	fmt.Println("##### Submitting the package into the repository")
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	taskUUID := raw["taskUUID"].(string)

	experimentalSuffix := "-experimental"
	if !raw["isExperimental"].(bool) {
		experimentalSuffix = ""
	}

	// Extract job info for notifications
	jobInfo := notification.JobNotificationInfo{
		PackageName:    raw["packageName"].(string),
		PackageVersion: raw["packageVersion"].(string),
		Maintainer:     raw["maintainer"].(string),
		IsExperimental: raw["isExperimental"].(bool),
	}
	if sourceURL, ok := raw["sourceUrl"].(string); ok {
		jobInfo.SourceURL = sourceURL
	}
	if sourceBranch, ok := raw["sourceBranch"].(string); ok {
		jobInfo.SourceBranch = sourceBranch
	}
	if packageURL, ok := raw["packageUrl"].(string); ok {
		jobInfo.PackageURL = packageURL
	}
	if packageBranch, ok := raw["packageBranch"].(string); ok {
		jobInfo.PackageBranch = packageBranch
	}

	logPath := irgshConfig.Repo.Workdir + "/artifacts/"
	logPath += taskUUID + "/repo.log"

	// Create the log file up front so that StreamLog has something to tail and
	// so early failures still end up in an uploadable log instead of being lost.
	if prepErr := systemutil.PrepareLogFile(logPath); prepErr != nil {
		fmt.Printf("error: unable to prepare log file %s: %v\n", logPath, prepErr)
		err = fmt.Errorf("unable to prepare log file %s: %w", logPath, prepErr)
		return
	}
	go systemutil.StreamLog(logPath)

	// Ensure notification is always sent on completion
	defer func() {
		if err != nil {
			sendRepoNotification(taskUUID, "FAILED", jobInfo)
		} else {
			sendRepoNotification(taskUUID, "SUCCESS", jobInfo)
		}
	}()

	artifactURL := fmt.Sprintf("%s/artifacts/%s.tar.gz", irgshConfig.Chief.Address, taskUUID)
	artifactDir := fmt.Sprintf("%s/artifacts", irgshConfig.Repo.Workdir)
	cmdStr := fmt.Sprintf(`mkdir -p %s && \
	cd %s/ && \
	wget --verbose --tries=3 --timeout=60 %s && \
	tar -xvf %s.tar.gz`,
		artifactDir,
		artifactDir,
		artifactURL,
		taskUUID,
	)
	_, err = systemutil.CmdExec(
		cmdStr,
		fmt.Sprintf("Downloading the artifact\nSource: %s\nDestination: %s/%s.tar.gz\nExtracting into: %s/%s",
			artifactURL, artifactDir, taskUUID, artifactDir, taskUUID),
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		systemutil.WriteLog(logPath, "[ REPO FAILED ] Failed to download artifact from "+artifactURL+": "+err.Error())
		systemutil.WriteLog(logPath, describeArtifactDir(artifactDir, taskUUID))
		uploadLog(logPath, taskUUID)
		return
	}
	systemutil.WriteLog(logPath, "##### Artifact downloaded and extracted successfully")
	systemutil.WriteLog(logPath, describeArtifactDir(artifactDir, taskUUID))

	gnupgDir := "GNUPGHOME=" + irgshConfig.Repo.GnupgDir
	if irgshConfig.IsDev {
		gnupgDir = ""
	}
	if raw["isExperimental"].(bool) {
		// Ignore version conflict
		cmdStr = fmt.Sprintf(`mkdir -p %s/%s && cd %s/%s/ && \
		%s reprepro -v -v -v --nothingiserror remove %s \
		$(cat %s/artifacts/%s/*.dsc | grep 'Source:' | cut -d ' ' -f 2)`,
			irgshConfig.Repo.Workdir,
			irgshConfig.Repo.DistCodename+experimentalSuffix,
			irgshConfig.Repo.Workdir,
			irgshConfig.Repo.DistCodename+experimentalSuffix,
			gnupgDir,
			irgshConfig.Repo.DistCodename+experimentalSuffix,
			irgshConfig.Repo.Workdir,
			taskUUID,
		)
		_, errExp := systemutil.CmdExec(
			cmdStr,
			"This is experimental package, remove any existing package.",
			logPath,
		)
		if errExp != nil {
			// Ignore err
			fmt.Printf("error: %v\n", errExp)
		}
	}

	// Handle force version - remove specific version before injecting
	forceVersion, ok := raw["forceVersion"].(bool)
	if ok && forceVersion && !raw["isExperimental"].(bool) {
		// Construct the full version string
		packageName := raw["packageName"].(string)
		packageVersion := raw["packageVersion"].(string)
		packageExtendedVersion, _ := raw["packageExtendedVersion"].(string)
		fullVersion := packageVersion
		if packageExtendedVersion != "" {
			fullVersion = packageVersion + "-" + packageExtendedVersion
		}

		// Remove the specific source version from the repository
		cmdStr = fmt.Sprintf(`mkdir -p %s/%s && cd %s/%s/ && \
		%s reprepro -v -v -v --nothingiserror removesrc %s %s %s`,
			irgshConfig.Repo.Workdir,
			irgshConfig.Repo.DistCodename+experimentalSuffix,
			irgshConfig.Repo.Workdir,
			irgshConfig.Repo.DistCodename+experimentalSuffix,
			gnupgDir,
			irgshConfig.Repo.DistCodename+experimentalSuffix,
			packageName,
			fullVersion,
		)
		_, errForce := systemutil.CmdExec(
			cmdStr,
			fmt.Sprintf("Force version: removing existing source package %s version %s", packageName, fullVersion),
			logPath,
		)
		if errForce != nil {
			// Ignore err - package might not exist yet
			fmt.Printf("error (ignored): %v\n", errForce)
		}

		// Remove the binary packages as well
		cmdStr = fmt.Sprintf(`mkdir -p %s/%s && cd %s/%s/ && \
		%s reprepro -v -v -v --nothingiserror remove %s %s`,
			irgshConfig.Repo.Workdir,
			irgshConfig.Repo.DistCodename+experimentalSuffix,
			irgshConfig.Repo.Workdir,
			irgshConfig.Repo.DistCodename+experimentalSuffix,
			gnupgDir,
			irgshConfig.Repo.DistCodename+experimentalSuffix,
			packageName,
		)
		_, errForce = systemutil.CmdExec(
			cmdStr,
			fmt.Sprintf("Force version: removing existing binary packages for %s", packageName),
			logPath,
		)
		if errForce != nil {
			// Ignore err - package might not exist yet
			fmt.Printf("error (ignored): %v\n", errForce)
		}
	}

	// Injecting the package
	cmdStr = fmt.Sprintf(`mkdir -p %s/%s && cd %s/%s/ && \
	%s reprepro -v -v -v --nothingiserror --component %s includedeb %s %s/artifacts/%s/*.deb`,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename+experimentalSuffix,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename+experimentalSuffix,
		gnupgDir,
		raw["component"],
		irgshConfig.Repo.DistCodename+experimentalSuffix,
		irgshConfig.Repo.Workdir,
		taskUUID,
	)

	_, err = systemutil.CmdExec(
		cmdStr,
		"Injecting the deb files from artifact to the repository",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		systemutil.WriteLog(logPath, "[ REPO FAILED ] Failed to inject deb files: "+err.Error())
		uploadLog(logPath, taskUUID)
		return
	}

	// Injecting source package via .dsc (avoids checksum mismatch between
	// CLI-built .deb and builder-built .deb that .changes would reference)
	ignoreDistribution := ""
	if raw["isExperimental"].(bool) {
		ignoreDistribution = "--ignore=wrongdistribution"
	}

	cmdStr = fmt.Sprintf(`mkdir -p %s/%s && cd %s/%s/ && \
	%s reprepro -v -v -v --nothingiserror %s --component %s includedsc %s %s/artifacts/%s/*.dsc`,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename+experimentalSuffix,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename+experimentalSuffix,
		gnupgDir,
		ignoreDistribution,
		raw["component"],
		irgshConfig.Repo.DistCodename+experimentalSuffix,
		irgshConfig.Repo.Workdir,
		taskUUID,
	)

	_, err = systemutil.CmdExec(
		cmdStr,
		"Injecting the dsc file from artifact to the repository",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		systemutil.WriteLog(logPath, "[ REPO FAILED ] Failed to inject dsc file: "+err.Error())
		uploadLog(logPath, taskUUID)
		return
	}

	cmdStr = fmt.Sprintf("mkdir -p %s/%s && cd %s/%s/ && %s reprepro -v -v -v export",
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		gnupgDir,
	)
	_, err = systemutil.CmdExec(
		cmdStr,
		"Re-export and publish the reprepro repository",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		systemutil.WriteLog(logPath, "[ REPO FAILED ] Failed to export repository: "+err.Error())
		uploadLog(logPath, taskUUID)
		return
	}

	systemutil.WriteLog(logPath, "[ REPO DONE ]")
	uploadLog(logPath, taskUUID)

	return
}

func InitRepo() (err error) {
	prompt := promptui.Prompt{
		Label:     "Are you sure you want to initialize new repository? Any existing distribution will be flushed.",
		IsConfirm: true,
	}
	result, err := prompt.Run()
	if err != nil {
		return
	}
	if strings.ToLower(result) != "y" {
		return
	}

	// TODO ask for matched distribution name as this command is super dangerous
	// Prepare workdir
	err = os.MkdirAll(irgshConfig.Repo.Workdir, 0755)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Println("##### Initializing new repository for " + irgshConfig.Repo.DistCodename)

	gnupgDir := "GNUPGHOME=" + irgshConfig.Repo.GnupgDir
	if irgshConfig.IsDev {
		gnupgDir = ""
	}

	logPath := irgshConfig.Repo.Workdir + "/init.log"
	go systemutil.StreamLog(logPath)

	repoTemplatePath := "/usr/share/irgsh/reprepro-template"
	if irgshConfig.IsDev {
		cwd, _ := os.Getwd()
		repoTemplatePath = cwd + "/utils/reprepro-template"
	} else if irgshConfig.IsTest {
		dir, _ := os.Getwd()
		repoTemplatePath = dir + "/../utils/reprepro-template"
	}
	cmdStr := fmt.Sprintf("mkdir -p %s && rm -rf %s/%s; cp -R %s %s/%s",
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		repoTemplatePath,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
	)
	_, err = systemutil.CmdExec(cmdStr, "Preparing reprepro template", logPath)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf(`mkdir -p %s/%s && cd %s/%s/conf && cat updates.orig | 
		sed 's/UPSTREAM_NAME/%s/g' | 
		sed 's/UPSTREAM_DIST_CODENAME/%s/g' | 
		sed 's/UPSTREAM_DIST_URL/%s/g' | 
		sed 's/DIST_SUPPORTED_ARCHITECTURES/%s/g' | 
		sed 's/UPSTREAM_DIST_COMPONENTS/%s/g' > updates && rm updates.orig`,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		irgshConfig.Repo.UpstreamName,
		irgshConfig.Repo.UpstreamDistCodename,
		strings.Replace(irgshConfig.Repo.UpstreamDistUrl, "/", "\\/", -1),
		irgshConfig.Repo.DistSupportedArchitectures,
		irgshConfig.Repo.UpstreamDistComponents,
	)
	_, err = systemutil.CmdExec(
		cmdStr,
		"Populate the reprepro's updates config file with values from irgsh's config.yaml",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf(`mkdir -p %s/%s/conf && cd %s/%s/conf && cat distributions.orig |
		sed 's/DIST_NAME/%s/g' |
		sed 's/DIST_LABEL/%s/g' |
		sed 's/DIST_CODENAME/%s/g' |
		sed 's/DIST_COMPONENTS/%s/g' |
		sed 's/DIST_SUPPORTED_ARCHITECTURES/%s/g' |
		sed 's/DIST_VERSION_DESC/%s/g' |
		sed 's/DIST_VERSION/%s/g' |
		sed 's/DIST_SIGNING_KEY/%s/g' |
		sed 's/UPSTREAM_NAME/%s/g'> distributions && rm distributions.orig`,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		irgshConfig.Repo.DistName,
		irgshConfig.Repo.DistLabel,
		irgshConfig.Repo.DistCodename,
		irgshConfig.Repo.DistComponents,
		irgshConfig.Repo.DistSupportedArchitectures,
		irgshConfig.Repo.DistVersionDesc,
		irgshConfig.Repo.DistVersion,
		irgshConfig.Repo.DistSigningKey,
		irgshConfig.Repo.UpstreamName,
	)
	_, err = systemutil.CmdExec(
		cmdStr,
		"Populate the reprepro's distributions config file with values from irgsh's config.yaml",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	repositoryPath := strings.Replace(
		irgshConfig.Repo.Workdir+"/"+irgshConfig.Repo.DistCodename,
		"/",
		"\\/",
		-1,
	)
	cmdStr = fmt.Sprintf(`mkdir -p %s/%s/conf && cd %s/%s/conf && \
	cat options.orig | sed 's/IRGSH_REPO_WORKDIR/%s/g' > options && \
	rm options.orig`,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		repositoryPath,
	)
	_, err = systemutil.CmdExec(
		cmdStr,
		"Populate the reprepro's options config file with values from irgsh's config.yaml",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf("cd %s/%s/ && %s reprepro -v -v -v export",
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		gnupgDir,
	)
	_, err = systemutil.CmdExec(
		cmdStr,
		"Re-export and publish the reprepro repository",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Println("##### Initializing the experimental repository for " + irgshConfig.Repo.DistCodename)
	// With -experimental suffix

	cmdStr = fmt.Sprintf("mkdir -p %s && rm -rf %s/%s; cp -R %s %s/%s",
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename+"-experimental",
		repoTemplatePath,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename+"-experimental",
	)
	_, err = systemutil.CmdExec(cmdStr, "Preparing reprepro template", logPath)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf(`cd %s/%s/conf && cat updates.orig | 
		sed 's/UPSTREAM_NAME/%s/g' | 
		sed 's/UPSTREAM_DIST_CODENAME/%s/g' | 
		sed 's/UPSTREAM_DIST_URL/%s/g' | 
		sed 's/DIST_SUPPORTED_ARCHITECTURES/%s/g' | 
		sed 's/UPSTREAM_DIST_COMPONENTS/%s/g' > updates && rm updates.orig`,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename+"-experimental",
		irgshConfig.Repo.UpstreamName,
		irgshConfig.Repo.UpstreamDistCodename+"-experimental",
		strings.Replace(irgshConfig.Repo.UpstreamDistUrl, "/", "\\/", -1),
		irgshConfig.Repo.DistSupportedArchitectures,
		irgshConfig.Repo.UpstreamDistComponents,
	)
	_, err = systemutil.CmdExec(
		cmdStr,
		"Populate the reprepro's updates config file with values from irgsh's config.yaml",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf(`cd %s/%s/conf && cat distributions.orig |
		sed 's/DIST_NAME/%s/g' |
		sed 's/DIST_LABEL/%s/g' |
		sed 's/DIST_CODENAME/%s/g' |
		sed 's/DIST_COMPONENTS/%s/g' |
		sed 's/DIST_SUPPORTED_ARCHITECTURES/%s/g' |
		sed 's/DIST_VERSION_DESC/%s/g' |
		sed 's/DIST_VERSION/%s/g' |
		sed 's/DIST_SIGNING_KEY/%s/g' |
		sed 's/UPSTREAM_NAME/%s/g'> distributions && rm distributions.orig`,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename+"-experimental",
		irgshConfig.Repo.DistName,
		irgshConfig.Repo.DistLabel,
		irgshConfig.Repo.DistCodename+"-experimental",
		irgshConfig.Repo.DistComponents,
		irgshConfig.Repo.DistSupportedArchitectures,
		irgshConfig.Repo.DistVersionDesc,
		irgshConfig.Repo.DistVersion,
		irgshConfig.Repo.DistSigningKey,
		irgshConfig.Repo.UpstreamName,
	)
	_, err = systemutil.CmdExec(
		cmdStr,
		"Populate the reprepro's distributions config file with values from irgsh's config.yaml",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	repositoryPath = strings.Replace(
		irgshConfig.Repo.Workdir+"/"+irgshConfig.Repo.DistCodename+"-experimental",
		"/",
		"\\/",
		-1,
	)
	cmdStr = fmt.Sprintf(`cd %s/%s/conf && \
	cat options.orig | sed 's/IRGSH_REPO_WORKDIR/%s/g' > options && \
	rm options.orig`,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename+"-experimental",
		repositoryPath,
	)
	_, err = systemutil.CmdExec(
		cmdStr,
		"Populate the reprepro's options config file with values from irgsh's config.yaml",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf("cd %s/%s/ && %s reprepro -v -v -v export",
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename+"-experimental",
		gnupgDir,
	)
	_, err = systemutil.CmdExec(
		cmdStr,
		"Re-export and publish the reprepro repository",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	return
}

func UpdateRepo() (err error) {
	fmt.Printf("Syncing irgshConfig.Repo.against %s at %s...",
		irgshConfig.Repo.UpstreamDistCodename,
		irgshConfig.Repo.UpstreamDistUrl,
	)

	logPath := irgshConfig.Repo.Workdir + "/update.log"
	go systemutil.StreamLog(logPath)

	cmdStr := fmt.Sprintf("cd %s/%s/ && reprepro -v -v -v update > %s",
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		logPath,
	)
	_, err = systemutil.CmdExec(cmdStr, "Sync the repository against upstream repository", logPath)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf("cd %s/%s/ && reprepro -v -v -v export",
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
	)
	_, err = systemutil.CmdExec(
		cmdStr,
		"Re-export and publish the reprepro repository",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	return
}
