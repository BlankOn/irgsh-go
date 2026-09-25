package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/blankon/irgsh-go/internal/logstream"
	"github.com/blankon/irgsh-go/internal/notification"
	"github.com/blankon/irgsh-go/pkg/systemutil"
)

func isoScriptPath() string {
	if path := os.Getenv("IRGSH_ISO_SCRIPT"); path != "" {
		return path
	}
	return "/usr/share/irgsh/iso-build.sh"
}

// ISOSubmission represents the payload for ISO build. The live-build
// repository URL is not part of it: that belongs to this worker's own config,
// the same way its distribution does.
type ISOSubmission struct {
	TaskUUID  string `json:"taskUUID"`
	Dist      string `json:"dist"`
	Branch    string `json:"branch"`
	NoCache   bool   `json:"noCache"`
	Timestamp string `json:"timestamp"`
}

// errCanceled ends a job that was cancelled by a maintainer. Machinery records
// it as a failure like any other error; chief reports the job as CANCELED from
// its own record of the cancellation.
var errCanceled = errors.New("job canceled on request")

func uploadLog(logPath string, id string) {
	// Upload the log to chief
	cmdStr := "curl -v -F 'uploadFile=@" + logPath + "' '"
	cmdStr += irgshConfig.Chief.Address + "/api/v1/log-upload?id=" + id + "&type=iso'"
	_, err := systemutil.CmdExec(
		cmdStr,
		"Uploading log file to chief",
		"",
	)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func sendISONotification(taskUUID, status string, jobInfo notification.JobNotificationInfo) {
	notification.SendJobNotification(
		irgshConfig.Notification.WebhookURL,
		"ISO Build",
		taskUUID,
		status,
		jobInfo,
	)
}

// writeBuildEnv writes the .env file the build script sources from its working
// directory. The script is deployment-agnostic; everything site-specific comes
// from this worker's config through here.
func writeBuildEnv(workdir string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "BUILD_JAHITAN_PATH=%q\n", irgshConfig.ISO.Outputdir)
	fmt.Fprintf(&b, "BUILD_PUBLISH_URL=%q\n", irgshConfig.ISO.PublicBaseURL)
	fmt.Fprintf(&b, "BUILD_LOCKFILE=%q\n", filepath.Join(workdir, "build-iso.lock"))
	fmt.Fprintf(&b, "TELEGRAM_BOT_KEY=%q\n", irgshConfig.ISO.TelegramBotKey)
	// 0600: this can carry a bot token.
	return os.WriteFile(filepath.Join(workdir, ".env"), []byte(b.String()), 0600)
}

// currentBuildID reports the build the output directory currently publishes,
// e.g. "20260904-1". A missing file is not an error: there may be no build yet.
func currentBuildID() string {
	data, err := os.ReadFile(filepath.Join(irgshConfig.ISO.Outputdir, "current", "current.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// BuildISO is the main ISO build task
func BuildISO(payload string) (next string, err error) {
	in := []byte(payload)
	var submission ISOSubmission
	err = json.Unmarshal(in, &submission)
	if err != nil {
		log.Printf("Failed to unmarshal payload: %v\n", err)
		return "", err
	}

	taskUUID := submission.TaskUUID
	fmt.Println("Processing ISO build pipeline: " + taskUUID)

	if submission.Dist != "" && submission.Dist != irgshConfig.ISO.DistCodename {
		return "", fmt.Errorf("iso task targeted dist %q but this iso instance serves %q",
			submission.Dist, irgshConfig.ISO.DistCodename)
	}
	if submission.Branch == "" {
		return "", fmt.Errorf("iso task has no branch")
	}

	// Extract job info for notifications
	jobInfo := notification.JobNotificationInfo{
		PackageName:  "ISO Image",
		Dist:         irgshConfig.ISO.DistCodename,
		SourceURL:    irgshConfig.ISO.RepoURL,
		SourceBranch: submission.Branch,
	}

	// The log is kept per task; the build itself runs in the shared workdir.
	artifactPath := filepath.Join(irgshConfig.ISO.Workdir, "artifacts", taskUUID)
	err = os.MkdirAll(artifactPath, 0755)
	if err != nil {
		log.Printf("Failed to create artifact directory: %v\n", err)
		return "", err
	}

	logPath := filepath.Join(artifactPath, "iso.log")
	if prepErr := systemutil.PrepareLogFile(logPath); prepErr != nil {
		log.Printf("error: unable to prepare log file %s: %v\n", logPath, prepErr)
	}
	stopLogStream := logstream.Mirror(logPublisher, taskUUID, "iso", logPath)
	defer stopLogStream()

	// Cancelling the job cancels this context, which stops live-build and
	// everything under it. The job may also be cancelled already, having been
	// stopped while it sat in the queue.
	job := cancelWatcher.Guard(taskUUID)
	defer job.Release()
	ctx := job.Context()

	// Set where the job actually stops, rather than read off job.Requested():
	// a request arriving after the build has finished changes nothing and
	// should not be reported as a cancellation.
	canceled := false

	// Ensure notification is always sent on completion
	defer func() {
		switch {
		case canceled:
			sendISONotification(taskUUID, "CANCELED", jobInfo)
		case err != nil:
			sendISONotification(taskUUID, "FAILED", jobInfo)
		default:
			sendISONotification(taskUUID, "SUCCESS", jobInfo)
		}
	}()

	// fail ends the job. A cancelled build has not failed: live-build was
	// killed on purpose, so its error describes the killing.
	fail := func(cause error) (string, error) {
		if job.Requested() {
			canceled = true
			systemutil.WriteLog(logPath, "[ ISO BUILD CANCELED ] The build was stopped on request")
			uploadLog(logPath, taskUUID)
			return "", errCanceled
		}
		systemutil.WriteLog(logPath, "[ ISO BUILD FAILED ] "+systemutil.FailureSummary(cause))
		uploadLog(logPath, taskUUID)
		return "", cause
	}

	if job.Requested() {
		err = errCanceled
		return fail(err)
	}

	scriptPath := isoScriptPath()
	if _, statErr := os.Stat(scriptPath); os.IsNotExist(statErr) {
		err = fmt.Errorf("iso-build.sh script not found at %s", scriptPath)
		return fail(err)
	}

	// The build runs in the configured workdir, a persistent live-build tree:
	// chroot/, cache/, auto/ and local/ are reused between builds so a rebuild
	// does not start from scratch.
	buildDir := irgshConfig.ISO.Workdir
	if err = writeBuildEnv(buildDir); err != nil {
		return fail(fmt.Errorf("unable to write build .env: %w", err))
	}

	if submission.NoCache {
		systemutil.WriteLog(logPath, "[ ISO BUILD ] Cacheless build requested, clearing cache, chroot, auto and local")
		cleanCmd := fmt.Sprintf("cd %s && sudo rm -rf cache chroot auto local", buildDir)
		if _, cleanErr := systemutil.CmdExecContext(ctx, cleanCmd, "Clearing live-build cache", logPath); cleanErr != nil {
			return fail(fmt.Errorf("unable to clear live-build directories: %w", cleanErr))
		}
	}

	systemutil.WriteLog(logPath, fmt.Sprintf(
		"[ ISO BUILD START ] Building ISO from %s branch %s in %s, output to %s",
		irgshConfig.ISO.RepoURL, submission.Branch, buildDir, irgshConfig.ISO.Outputdir))

	// What the output directory published before this build. The script only
	// advances it on success, so comparing afterwards distinguishes a fresh
	// build from a stale one left by an earlier job.
	buildIDBefore := currentBuildID()

	// Execute: sudo iso-build.sh <repo-url> <branch>, in the shared workdir.
	// pipefail so the script's exit code survives the pipe into tee.
	cmdStr := fmt.Sprintf("cd %s && set -o pipefail && yes | sudo %s %s %s 2>&1 | tee -a %s",
		buildDir, scriptPath, irgshConfig.ISO.RepoURL, submission.Branch, logPath)

	log.Println("Executing: " + cmdStr)
	_, err = systemutil.CmdExecContext(
		ctx,
		cmdStr,
		"Building ISO image",
		logPath,
	)
	if err != nil {
		return fail(fmt.Errorf("build failed: %w", err))
	}

	// Verify a *new* ISO was published. Checking only that current/*.iso
	// exists would pass on the previous build's output whenever this one
	// failed without replacing it.
	buildIDAfter := currentBuildID()
	if buildIDAfter == "" || buildIDAfter == buildIDBefore {
		err = fmt.Errorf("no new ISO was published in %s/current (still %q)",
			irgshConfig.ISO.Outputdir, buildIDBefore)
		return fail(err)
	}

	isoFiles, globErr := filepath.Glob(filepath.Join(irgshConfig.ISO.Outputdir, "current", "*.iso"))
	if globErr != nil {
		err = fmt.Errorf("failed to search for ISO files: %v", globErr)
		return fail(err)
	}
	if len(isoFiles) == 0 {
		err = fmt.Errorf("no ISO file found in %s/current/", irgshConfig.ISO.Outputdir)
		return fail(err)
	}

	log.Printf("ISO file(s) found: %v\n", isoFiles)
	systemutil.WriteLog(logPath, fmt.Sprintf("[ ISO BUILD DONE ] %s published as %s", isoFiles[0], buildIDAfter))
	uploadLog(logPath, taskUUID)

	fmt.Println("ISO build done.")
	next = payload
	return
}
