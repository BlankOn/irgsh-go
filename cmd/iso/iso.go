package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/blankon/irgsh-go/internal/notification"
	"github.com/blankon/irgsh-go/pkg/systemutil"
)

// ISOSubmission represents the payload for ISO build
type ISOSubmission struct {
	TaskUUID  string `json:"taskUUID"`
	RepoURL   string `json:"repoUrl"`
	Branch    string `json:"branch"`
	Timestamp string `json:"timestamp"`
}

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

	// Extract job info for notifications
	jobInfo := notification.JobNotificationInfo{
		PackageName: "ISO Image",
		SourceURL:   submission.RepoURL,
		SourceBranch: submission.Branch,
	}

	// Create artifacts directory
	artifactPath := irgshConfig.ISO.Workdir + "/artifacts/" + taskUUID
	err = os.MkdirAll(artifactPath, 0755)
	if err != nil {
		log.Printf("Failed to create artifact directory: %v\n", err)
		return "", err
	}

	logPath := artifactPath + "/iso.log"
	go systemutil.StreamLog(logPath)

	// Ensure notification is always sent on completion
	defer func() {
		if err != nil {
			sendISONotification(taskUUID, "FAILED", jobInfo)
		} else {
			sendISONotification(taskUUID, "SUCCESS", jobInfo)
		}
	}()

	// Run the iso-build.sh script from /usr/share/irgsh/
	scriptPath := "/usr/share/irgsh/iso-build.sh"

	// Check if script exists
	if _, statErr := os.Stat(scriptPath); os.IsNotExist(statErr) {
		err = fmt.Errorf("iso-build.sh script not found at %s", scriptPath)
		systemutil.WriteLog(logPath, "[ ISO BUILD FAILED ] "+err.Error())
		uploadLog(logPath, taskUUID)
		return "", err
	}

	systemutil.WriteLog(logPath, fmt.Sprintf("[ ISO BUILD START ] Building ISO from %s branch %s, output to %s", submission.RepoURL, submission.Branch, irgshConfig.ISO.Outputdir))

	// Execute: sudo ./iso-build.sh repo-url branch-name outputdir
	// Use pipefail to capture the script's exit code even when piping to tee
	cmdStr := fmt.Sprintf("cd %s && set -o pipefail && sudo %s %s %s %s 2>&1 | tee -a %s",
		artifactPath, scriptPath, submission.RepoURL, submission.Branch, irgshConfig.ISO.Outputdir, logPath)

	log.Println("Executing: " + cmdStr)
	_, err = systemutil.CmdExec(
		cmdStr,
		"Building ISO image",
		logPath,
	)
	if err != nil {
		systemutil.WriteLog(logPath, "[ ISO BUILD FAILED ] Build failed: "+err.Error())
		uploadLog(logPath, taskUUID)
		return "", err
	}

	// Verify ISO file was created in the output directory
	// The script creates a "current" symlink pointing to the latest build
	isoPattern := filepath.Join(irgshConfig.ISO.Outputdir, "current", "*.iso")
	isoFiles, globErr := filepath.Glob(isoPattern)
	if globErr != nil {
		err = fmt.Errorf("failed to search for ISO files: %v", globErr)
		systemutil.WriteLog(logPath, "[ ISO BUILD FAILED ] "+err.Error())
		uploadLog(logPath, taskUUID)
		return "", err
	}

	if len(isoFiles) == 0 {
		err = fmt.Errorf("no ISO file found in %s/current/", irgshConfig.ISO.Outputdir)
		systemutil.WriteLog(logPath, "[ ISO BUILD FAILED ] "+err.Error())
		uploadLog(logPath, taskUUID)
		return "", err
	}

	log.Printf("ISO file(s) found: %v\n", isoFiles)
	systemutil.WriteLog(logPath, fmt.Sprintf("[ ISO BUILD DONE ] ISO file created: %s", isoFiles[0]))
	uploadLog(logPath, taskUUID)

	fmt.Println("ISO build done.")
	next = payload
	return
}
