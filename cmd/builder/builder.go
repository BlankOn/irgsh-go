package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/blankon/irgsh-go/internal/logstream"
	"github.com/blankon/irgsh-go/internal/notification"
	"github.com/blankon/irgsh-go/pkg/systemutil"
)

// errCanceled ends a job that was cancelled by a maintainer. Machinery
// records it as a failure like any other error; chief reports the job as
// CANCELED from its own record of the cancellation.
var errCanceled = errors.New("job canceled on request")

func uploadLog(logPath string, id string) {
	// Upload the log to chief
	cmdStr := "curl -v -F 'uploadFile=@" + logPath + "' '"
	cmdStr += irgshConfig.Chief.Address + "/api/v1/log-upload?id=" + id + "&type=build'"
	_, err := systemutil.CmdExec(
		cmdStr,
		"Uploading log file to chief",
		"",
	)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func sendBuildNotification(taskUUID, status string, jobInfo notification.JobNotificationInfo) {
	notification.SendJobNotification(
		irgshConfig.Notification.WebhookURL,
		"Build",
		taskUUID,
		status,
		jobInfo,
	)
}

// Main task wrapper
func Build(payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	taskUUID := raw["taskUUID"].(string)
	fmt.Println("Processing pipeline :" + taskUUID)

	if dist, ok := raw["dist"].(string); ok && dist != "" && dist != irgshConfig.Builder.DistCodename {
		return "", fmt.Errorf("build task targeted dist %q but this builder instance serves %q",
			dist, irgshConfig.Builder.DistCodename)
	}

	// Extract job info for notifications
	jobInfo := notification.JobNotificationInfo{
		PackageName:    raw["packageName"].(string),
		PackageVersion: raw["packageVersion"].(string),
		Maintainer:     raw["maintainer"].(string),
		Dist:           irgshConfig.Builder.DistCodename,
		IsExperimental: raw["isExperimental"].(bool),
	}
	if component, ok := raw["component"].(string); ok {
		jobInfo.Component = component
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

	logPath := irgshConfig.Builder.Workdir + "/artifacts/" + taskUUID + "/build.log"
	if prepErr := systemutil.PrepareLogFile(logPath); prepErr != nil {
		log.Printf("error: unable to prepare log file %s: %v\n", logPath, prepErr)
	}
	stopLogStream := logstream.Mirror(logPublisher, taskUUID, "build", logPath)
	defer stopLogStream()

	// Cancelling the job cancels this context, which kills every command the
	// build is running. The job may also already be cancelled here, having
	// been stopped while it sat in the queue.
	job := cancelWatcher.Guard(taskUUID)
	defer job.Release()
	ctx := job.Context()

	// Set where the job actually stops, rather than read off job.Requested():
	// a request arriving after the last step has finished changes nothing and
	// should not be reported as a cancellation.
	canceled := false

	// Ensure notification is always sent on completion
	defer func() {
		switch {
		case canceled:
			sendBuildNotification(taskUUID, "CANCELED", jobInfo)
		case err != nil:
			sendBuildNotification(taskUUID, "FAILED", jobInfo)
		default:
			sendBuildNotification(taskUUID, "SUCCESS", jobInfo)
		}
	}()

	// finish ends the job. A cancelled build has not failed: the commands it
	// was running were killed on purpose, so whatever error they returned
	// describes the killing rather than the package.
	finish := func(stage string, cause error) (string, error) {
		if job.Requested() {
			canceled = true
			systemutil.WriteLog(logPath, "[ BUILD CANCELED ] "+stage+" was stopped on request")
			uploadLog(logPath, taskUUID)
			return "", errCanceled
		}
		systemutil.WriteLog(logPath, "[ BUILD FAILED ] "+stage+" failed: "+systemutil.FailureSummary(cause))
		uploadLog(logPath, taskUUID)
		return "", cause
	}

	if job.Requested() {
		return finish("The build", errCanceled)
	}

	next, err = BuildPreparation(ctx, payload)
	if err != nil {
		return finish("Build preparation", err)
	}

	next, err = BuildPackage(ctx, payload)
	if err != nil {
		return finish("Package build", err)
	}

	next, err = StorePackage(ctx, payload)
	if err != nil {
		return finish("Package artifact upload", err)
	}

	systemutil.WriteLog(logPath, "[ BUILD DONE ]")
	uploadLog(logPath, taskUUID)

	fmt.Println("Done.")

	return
}

func BuildPreparation(ctx context.Context, payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	buildPath := irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
	logPath := buildPath + "/build.log"

	targetDir := irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
	err = os.MkdirAll(targetDir, 0755)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	target := irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string) + "/debuild.tar.gz"
	// Downloading the submission tarball from chief
	cmdStr := "curl -v -o " + target + " "
	cmdStr += irgshConfig.Chief.Address + "/submissions/" + raw["taskUUID"].(string) + ".tar.gz"
	log.Println(cmdStr)
	_, err = systemutil.CmdExecContext(
		ctx,
		cmdStr,
		"Fetching the submission tarball from chief",
		logPath,
	)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	// Extract the signed dsc
	cmdStr = "cd " + irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
	cmdStr += " && tar -xvf debuild.tar.gz "
	cmdStr += " && rm -rf debuild.tar.gz "
	_, err = systemutil.CmdExecContext(
		ctx,
		cmdStr,
		"Backup the maintainer tarball and its signature",
		logPath,
	)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	next = payload
	return
}

func BuildPackage(ctx context.Context, payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	buildPath := irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
	err = os.MkdirAll(buildPath, 0755)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	logPath := buildPath + "/build.log"

	packageNameVersion := raw["packageName"].(string) + "-" + raw["packageVersion"].(string)
	if len(raw["packageExtendedVersion"].(string)) > 0 {
		packageNameVersion += "-" + raw["packageExtendedVersion"].(string)
	}

	// Copy the maintainer's generated files from signed dir
	cmdStr := "cd " + irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
	cmdStr += " && cp signed/* ."
	log.Println(cmdStr)
	_, err = systemutil.CmdExecContext(
		ctx,
		cmdStr,
		"Copy the maintainer's generated files from signed dir.",
		logPath,
	)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	// Cleanup pbuilder cache result
	_, _ = systemutil.CmdExecContext(
		ctx,
		"rm -rf /var/cache/pbuilder/result/*",
		"",
		"",
	)

	// Cancelling the job kills the docker client, which by itself leaves the
	// container it started running: the build would carry on inside it with
	// nothing left watching. The container id is written out so it can be
	// removed either way.
	cidPath := buildPath + "/container.cid"
	// docker refuses to start when the cid file already exists, which a
	// retried task would find left behind.
	_ = os.Remove(cidPath)
	defer removeBuildContainer(cidPath)

	// Building the package
	// BUILD_ATTEMPTS lets /build.sh retry a build that failed on a transient
	// network/DNS error; see builder/init.go to modify that script.
	cmdStr = "docker run -v " + irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
	cmdStr += ":/tmp/build --privileged=true --user 0:0"
	cmdStr += " -e BUILD_ATTEMPTS=" + strconv.Itoa(irgshConfig.Builder.Attempts())
	cmdStr += " --cidfile " + cidPath
	cmdStr += " -i pbocker bash -c /build.sh"
	fmt.Println(cmdStr)
	_, err = systemutil.CmdExecContext(
		ctx,
		cmdStr,
		"Building the package",
		logPath,
	)
	if err != nil {
		log.Println(err.Error())
		return
	}

	// Check if .deb files were created
	debPattern := irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string) + "/*.deb"
	debFiles, _ := filepath.Glob(debPattern)
	if len(debFiles) == 0 {
		err = fmt.Errorf("no .deb files were created after build")
		log.Println(err.Error())
		return
	}
	log.Printf("Found %d .deb file(s): %v\n", len(debFiles), debFiles)

	// Use the generated files from maintainer
	if len(raw["sourceUrl"].(string)) > 0 {
		cmdStr := "cd " + irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
		cmdStr += " && cp signed/* . "
		log.Println(cmdStr)
		_, err = systemutil.CmdExecContext(
			ctx,
			cmdStr,
			"Use the generated files from maintainer.",
			logPath,
		)
		if err != nil {
			log.Printf("error: %v\n", err)
			return
		}
	}

	next = payload
	return
}

// removeBuildContainer removes the container named by a cid file, and the file
// with it. A build that ran to its end has already exited, so this is really
// for a cancelled one, whose container outlives the docker client that was
// killed. It deliberately runs outside the job's context, which is exactly the
// one that has just been cancelled.
func removeBuildContainer(cidPath string) {
	cid, err := os.ReadFile(cidPath)
	if err == nil {
		if id := string(bytes.TrimSpace(cid)); id != "" {
			if _, rmErr := systemutil.CmdExec("docker rm -f "+id, "", ""); rmErr != nil {
				log.Printf("error: unable to remove build container %s: %v\n", id, rmErr)
			}
		}
	}
	_ = os.Remove(cidPath)
}

func StorePackage(ctx context.Context, payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	logPath := irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string) + "/build.log"

	cmdStr := "cd " + irgshConfig.Builder.Workdir + "/artifacts/ && "
	// build.log is excluded because tar's own verbose output is being appended
	// to it while tar reads it, which makes tar exit non-zero with "file
	// changed as we read it" and masks the real build result. Chief receives
	// the log through the separate log-upload endpoint anyway.
	cmdStr += "tar --exclude=build.log -zcvf " + raw["taskUUID"].(string) + ".tar.gz " + raw["taskUUID"].(string)
	cmdStr += " && curl -v -F 'uploadFile=@" + irgshConfig.Builder.Workdir
	cmdStr += "/artifacts/" + raw["taskUUID"].(string) + ".tar.gz' "
	cmdStr += irgshConfig.Chief.Address + "/api/v1/artifact-upload?id="
	cmdStr += raw["taskUUID"].(string)
	_, err = systemutil.CmdExecContext(
		ctx,
		cmdStr,
		"",
		logPath,
	)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	next = payload
	return
}
