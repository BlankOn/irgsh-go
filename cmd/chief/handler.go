package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/RichardKnop/machinery/v1/backends/result"
	"github.com/RichardKnop/machinery/v1/tasks"
	"github.com/google/uuid"

	"github.com/blankon/irgsh-go/internal/monitoring"
)

// Maintainer represents a GPG key maintainer
type Maintainer struct {
	KeyID string
	Name  string
	Email string
}

// getMaintainers parses GPG keys and returns a list of maintainers
func getMaintainers() []Maintainer {
	gnupgDir := "GNUPGHOME=" + irgshConfig.Chief.GnupgDir
	if irgshConfig.IsDev {
		gnupgDir = ""
	}

	// Use --with-colons for easier parsing
	cmdStr := gnupgDir + " gpg --list-keys --with-colons"

	output, err := exec.Command("bash", "-c", cmdStr).Output()
	if err != nil {
		log.Printf("Failed to list GPG keys: %v\n", err)
		return []Maintainer{}
	}

	return parseGPGKeys(string(output))
}

// parseGPGKeys parses GPG --with-colons output
func parseGPGKeys(output string) []Maintainer {
	var maintainers []Maintainer
	var currentKey *Maintainer

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		fields := strings.Split(line, ":")
		if len(fields) < 2 {
			continue
		}

		recordType := fields[0]

		switch recordType {
		case "pub": // Public key
			if currentKey != nil {
				// Save previous key if we have one
				maintainers = append(maintainers, *currentKey)
			}
			// Start new key
			currentKey = &Maintainer{
				KeyID: "",
				Name:  "",
				Email: "",
			}

			// Key ID is in field 4 (short key ID, last 8 chars of field 4)
			if len(fields) > 4 && len(fields[4]) >= 8 {
				currentKey.KeyID = fields[4][len(fields[4])-16:] // Last 16 chars (short key ID)
			}

		case "uid": // User ID
			if currentKey != nil && len(fields) > 9 {
				// Parse "Name <email>" format from field 9
				uid := fields[9]

				// Extract name and email
				if strings.Contains(uid, "<") && strings.Contains(uid, ">") {
					parts := strings.SplitN(uid, "<", 2)
					currentKey.Name = strings.TrimSpace(parts[0])
					if len(parts) > 1 {
						emailPart := strings.SplitN(parts[1], ">", 2)
						currentKey.Email = strings.TrimSpace(emailPart[0])
					}
				} else {
					// No email, just name
					currentKey.Name = uid
				}
			}
		}
	}

	// Don't forget the last key
	if currentKey != nil {
		maintainers = append(maintainers, *currentKey)
	}

	return maintainers
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta http-equiv="refresh" content="10">
    <title>IRGSH Chief</title>
    <style>
        body {
            font-family: monospace;
            margin: 20px;
            background-color: #f5f5f5;
        }
        .header {
            background: #333;
            color: #fff;
            padding: 15px;
            margin-bottom: 20px;
        }
        .logo {
            font-size: 14px;
            line-height: 1.2;
            margin-bottom: 10px;
        }
        .nav {
            margin-top: 10px;
        }
        .nav a {
            color: #4CAF50;
            text-decoration: none;
            margin-right: 10px;
        }
        .nav a:hover {
            text-decoration: underline;
        }
        .summary {
            background: #fff;
            padding: 15px;
            margin-bottom: 20px;
            border-left: 4px solid #4CAF50;
            display: inline-block;
        }
        .summary-item {
            display: inline-block;
            margin-right: 30px;
            font-size: 14px;
            vertical-align: top;
        }
        .summary-number {
            font-size: 24px;
            font-weight: bold;
            color: #4CAF50;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            background: #fff;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);
        }
        th {
            background: #333;
            color: #fff;
            padding: 12px;
            text-align: left;
            font-size: 12px;
        }
        td {
            padding: 10px 12px;
            border-bottom: 1px solid #ddd;
            font-size: 11px;
        }
        tr:hover {
            background-color: #f9f9f9;
        }
        .status-online {
            color: #4CAF50;
            font-weight: bold;
        }
        .status-offline {
            color: #f44336;
            font-weight: bold;
        }
        .status-warning {
            color: #ff9800;
            font-weight: bold;
        }
        .status-stalled {
            color: #e91e63;
            font-weight: bold;
        }
        .badge {
            display: inline-block;
            padding: 3px 8px;
            border-radius: 3px;
            font-size: 10px;
            font-weight: bold;
        }
        .badge-builder {
            background: #2196F3;
            color: white;
        }
        .badge-repo {
            background: #FF9800;
            color: white;
        }
        .badge-iso {
            background: #9C27B0;
            color: white;
        }
        .metric {
            font-size: 11px;
            color: #666;
        }
        .section-title {
            font-size: 16px;
            font-weight: bold;
            margin: 20px 0 10px 0;
            color: #333;
        }
        .refresh-info {
            color: #666;
            font-size: 11px;
            margin-top: 20px;
        }
        .empty-state {
            background: #fff;
            padding: 40px;
            text-align: center;
            color: #999;
        }
    </style>
</head>
<body>
    <div class="header">
        <div>irgsh-chief ` + app.Version + `</div>
    </div>
`

	// Add Package Maintainers section first
	html += `<div class="section-title">Package Maintainers</div>`

	maintainers := getMaintainers()
	if len(maintainers) > 0 {
		html += `
		<table>
			<thead>
				<tr>
					<th>GPG Key</th>
					<th>Name</th>
					<th>Email</th>
				</tr>
			</thead>
			<tbody>`

		for _, m := range maintainers {
			html += fmt.Sprintf(`
				<tr>
					<td style="font-family: monospace;">%s</td>
					<td>%s</td>
					<td>%s</td>
				</tr>`,
				m.KeyID,
				m.Name,
				m.Email,
			)
		}

		html += `
			</tbody>
		</table>`
	} else {
		html += `<div class="empty-state">No maintainers found</div>`
	}

	// Add monitoring section if enabled
	if irgshConfig.Monitoring.Enabled && monitoringRegistry != nil {
		// Get all instances first (Workers section)
		instances, err := monitoringRegistry.ListInstances("", "")
		if err != nil {
			log.Printf("Failed to list instances: %v\n", err)
		} else {
			// Get summary
			summary, err := monitoringRegistry.GetSummary()
			if err != nil {
				log.Printf("Failed to get summary: %v\n", err)
			}

			html += `<div class="section-title">Workers</div>`

			// Summary section
			html += fmt.Sprintf(`
    <div class="summary">
        <div class="summary-item">
            <div class="summary-number">%d</div>
            <div>Total Instances</div>
        </div>
        <div class="summary-item">
            <div class="summary-number" style="color: #4CAF50;">%d</div>
            <div>Online</div>
        </div>
        <div class="summary-item">
            <div class="summary-number" style="color: #f44336;">%d</div>
            <div>Offline</div>
        </div>
`, summary.Total, summary.Online, summary.Offline)

			// Add type breakdown
			for typeName, count := range summary.ByType {
				html += fmt.Sprintf(`
        <div class="summary-item">
            <div class="summary-number" style="color: #2196F3;">%d</div>
            <div>%s</div>
        </div>
`, count, typeName)
			}

			html += `
    </div>
`

			// Instance table
			if len(instances) > 0 {
				html += `
    <table>
        <thead>
            <tr>
                <th>Instance</th>
                <th>Type</th>
                <th>Hostname</th>
                <th>Status</th>
                <th>Uptime</th>
                <th>Tasks</th>
                <th>CPU</th>
                <th>Memory</th>
                <th>Disk</th>
            </tr>
        </thead>
        <tbody>`

				for _, instance := range instances {
					// Type badge
					badgeClass := "badge badge-builder"
					switch instance.InstanceType {
					case monitoring.InstanceTypeRepo:
						badgeClass = "badge badge-repo"
					case monitoring.InstanceTypeISO:
						badgeClass = "badge badge-iso"
					}

					// Status class
					statusClass := "status-offline"
					if instance.Status == monitoring.StatusOnline {
						statusClass = "status-online"
					}

					// Calculate uptime
					uptime := time.Since(instance.StartTime)
					uptimeStr := formatDuration(uptime)

					// Format CPU
					cpuStr := fmt.Sprintf("%.1f", instance.CPUUsage)

					// Format memory
					memStr := monitoring.FormatBytes(instance.MemoryUsage)
					if instance.MemoryTotal > 0 {
						memStr += " / " + monitoring.FormatBytes(instance.MemoryTotal)
					}

					// Format disk
					diskStr := monitoring.FormatBytes(instance.DiskUsage)
					if instance.DiskTotal > 0 {
						diskStr += " / " + monitoring.FormatBytes(instance.DiskTotal)
					}

					html += fmt.Sprintf(`
            <tr>
                <td style="font-family: monospace; font-size: 0.85em;">%s</td>
                <td><span class="%s">%s</span></td>
                <td>%s</td>
                <td><span class="%s">%s</span></td>
                <td>%s</td>
                <td>%d / %d</td>
                <td class="metric">%s / 100</td>
                <td class="metric">%s</td>
                <td class="metric">%s</td>
            </tr>`,
						instance.InstanceID,
						badgeClass,
						instance.InstanceType,
						instance.Hostname,
						statusClass,
						instance.Status,
						uptimeStr,
						instance.ActiveTasks,
						instance.Concurrency,
						cpuStr,
						memStr,
						diskStr,
					)
				}

				html += `
        </tbody>
    </table>
`
			} else {
				html += `<div class="empty-state">No worker instances found</div>`
			}
		}

		// Check if builder and repo instances are online (for stalled job detection)
		hasOnlineBuilder := false
		hasOnlineRepo := false
		for _, instance := range instances {
			if instance.Status == monitoring.StatusOnline {
				if instance.InstanceType == monitoring.InstanceTypeBuilder {
					hasOnlineBuilder = true
				} else if instance.InstanceType == monitoring.InstanceTypeRepo {
					hasOnlineRepo = true
				}
			}
		}
		workersOnline := hasOnlineBuilder && hasOnlineRepo

		// Get recent jobs (Recent Jobs section)
		jobs, err := monitoringRegistry.GetRecentJobs(10)
		if err != nil {
			log.Printf("Failed to list jobs: %v\n", err)
		} else if len(jobs) > 0 {
			html += `<div class="section-title">Recent Jobs</div>`
			html += `
			<table>
				<thead>
					<tr>
						<th>Timestamp</th>
						<th>Package</th>
						<th>Version</th>
						<th>Maintainer</th>
						<th>Component</th>
						<th>Status</th>
						<th>Logs</th>
						<th>UUID</th>
					</tr>
				</thead>
				<tbody>`

			// Get actual task states from machinery
			for _, job := range jobs {
				// Query both build and repo task states
				buildState, repoState, currentStage := monitoring.GetJobStagesFromMachinery(
					monitoringRegistry.GetContext(),
					monitoringRegistry.GetClient(),
					job.TaskUUID,
				)

				// Update job with stage information
				job.BuildState = buildState
				job.RepoState = repoState
				job.CurrentStage = currentStage

				// Determine overall state
				overallState := "PENDING"
				if buildState == "FAILURE" {
					overallState = "FAILURE"
				} else if repoState == "FAILURE" {
					overallState = "FAILURE"
				} else if buildState == "SUCCESS" && repoState == "SUCCESS" {
					overallState = "SUCCESS"
				} else if buildState == "STARTED" || repoState == "STARTED" {
					overallState = "STARTED"
				}

				job.State = overallState

				// Determine status color and text
				statusClass := ""
				statusText := overallState
				switch overallState {
				case "SUCCESS":
					statusClass = "status-online"
				case "FAILURE":
					statusClass = "status-offline"
					// Show which stage failed
					if buildState == "FAILURE" {
						statusText = "FAILURE (build)"
					} else if repoState == "FAILURE" {
						statusText = "FAILURE (repo)"
					}
				case "STARTED":
					statusClass = "status-warning"
					// Show which stage is running
					statusText = "STARTED (" + currentStage + ")"
				default:
					// Check if job is stalled (PENDING for > 5 minutes while workers are online)
					if workersOnline && time.Since(job.SubmittedAt) > 5*time.Minute {
						statusClass = "status-stalled"
						statusText = "STALLED"
					} else {
						statusText = "PENDING"
					}
				}

				// Format timestamp in RFC3339
				timeStr := job.SubmittedAt.Format(time.RFC3339)

				expTag := ""
				if job.IsExperimental {
					expTag = " <span style=\"color: #ff9800; font-weight: bold;\">[experimental]</span>"
				}

				// Build package cell with git repository links if available
				packageCell := job.PackageName + expTag
				var repoLinks []string
				if job.SourceURL != "" {
					branchText := job.SourceBranch
					if branchText == "" {
						branchText = "default"
					}
					repoLinks = append(repoLinks, fmt.Sprintf(`<a href="%s" target="_blank">source (%s)</a>`, job.SourceURL, branchText))
				}
				if job.PackageURL != "" {
					branchText := job.PackageBranch
					if branchText == "" {
						branchText = "default"
					}
					repoLinks = append(repoLinks, fmt.Sprintf(`<a href="%s" target="_blank">package (%s)</a>`, job.PackageURL, branchText))
				}
				if len(repoLinks) > 0 {
					packageCell += fmt.Sprintf(`<br><span style="font-size: 0.85em; color: #666;">%s</span>`,
						strings.Join(repoLinks, ", "))
				}

				html += fmt.Sprintf(`
					<tr>
						<td>%s</td>
						<td>%s</td>
						<td>%s</td>
						<td>%s</td>
						<td>%s</td>
						<td><span class="%s">%s</span></td>
						<td>
							<a href="/logs/%s.build.log" target="_blank">build.log</a> |
							<a href="/logs/%s.repo.log" target="_blank">repo.log</a>
						</td>
						<td style="font-family: monospace; font-size: 0.85em;">%s</td>
					</tr>`,
					timeStr,
					packageCell,
					job.PackageVersion,
					job.Maintainer,
					job.Component,
					statusClass,
					statusText,
					job.TaskUUID,
					job.TaskUUID,
					job.TaskUUID,
				)
			}

			html += `
				</tbody>
			</table>
			`
		}
	}

	html += `
    <div class="refresh-info">
        Page auto-refreshes every 10 seconds
    </div>
</body>
</html>
`

	fmt.Fprintf(w, html)
}

func PackageSubmitHandler(w http.ResponseWriter, r *http.Request) {
	submission := Submission{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&submission)
	if err != nil {
		fmt.Println(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "400")
		return
	}
	submission.Timestamp = time.Now()
	submission.TaskUUID = submission.Timestamp.Format("2006-01-02-150405") + "_" + uuid.New().String() + "_" + submission.MaintainerFingerprint + "_" + submission.PackageName

	// Verifying the signature against current gpg keyring
	cmdStr := "mkdir -p " + irgshConfig.Chief.Workdir + "/submissions/" + submission.TaskUUID
	fmt.Println(cmdStr)
	cmd := exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}

	src := irgshConfig.Chief.Workdir + "/submissions/" + submission.Tarball + ".tar.gz"
	path := irgshConfig.Chief.Workdir + "/submissions/" + submission.TaskUUID + ".tar.gz"
	err = Move(src, path)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}

	cmdStr = "cd " + irgshConfig.Chief.Workdir + "/submissions/ "
	cmdStr += " && tar -xvf " + submission.TaskUUID + ".tar.gz -C " + submission.TaskUUID
	fmt.Println(cmdStr)
	err = exec.Command("bash", "-c", cmdStr).Run()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}

	src = irgshConfig.Chief.Workdir + "/submissions/" + submission.Tarball + ".token"
	path = irgshConfig.Chief.Workdir + "/submissions/" + submission.TaskUUID + ".sig.txt"
	err = Move(src, path)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}

	gnupgDir := "GNUPGHOME=" + irgshConfig.Chief.GnupgDir
	if irgshConfig.IsDev {
		gnupgDir = ""
	}

	cmdStr = "cd " + irgshConfig.Chief.Workdir + "/submissions/" + submission.TaskUUID + " && "
	cmdStr += gnupgDir + " gpg --verify signed/*.dsc"
	err = exec.Command("bash", "-c", cmdStr).Run()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "401 Unauthorized")
		return
	}

	jsonStr, err := json.Marshal(submission)
	if err != nil {
		fmt.Println(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "400")
		return
	}

	buildSignature := tasks.Signature{
		Name: "build",
		UUID: submission.TaskUUID,
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: string(jsonStr),
			},
		},
	}

	repoSignature := tasks.Signature{
		Name: "repo",
		UUID: submission.TaskUUID,
	}

	chain, _ := tasks.NewChain(&buildSignature, &repoSignature)
	_, err = server.SendChain(chain)
	if err != nil {
		fmt.Println("Could not send chain : " + err.Error())
	}

	// Record job in monitoring system
	if irgshConfig.Monitoring.Enabled && monitoringRegistry != nil {
		job := monitoring.JobInfo{
			TaskUUID:       submission.TaskUUID,
			PackageName:    submission.PackageName,
			PackageVersion: submission.PackageVersion,
			Maintainer:     submission.Maintainer,
			Component:      submission.Component,
			IsExperimental: submission.IsExperimental,
			SubmittedAt:    submission.Timestamp,
			State:          "PENDING",
			PackageURL:     submission.PackageURL,
			SourceURL:      submission.SourceURL,
			PackageBranch:  submission.PackageBranch,
			SourceBranch:   submission.SourceBranch,
		}
		if err := monitoringRegistry.RecordJob(job); err != nil {
			log.Printf("Failed to record job: %v\n", err)
		}
	}

	payload := SubmitPayloadResponse{PipelineId: submission.TaskUUID}
	jsonStr, _ = json.Marshal(payload)
	fmt.Fprintf(w, string(jsonStr))

}

func BuildStatusHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["uuid"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "403")
		return
	}
	var UUID string
	UUID = keys[0]

	// Check build task state
	buildSignature := tasks.Signature{
		Name: "build",
		UUID: UUID,
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: "xyz",
			},
		},
	}
	buildResult := result.NewAsyncResult(&buildSignature, server.GetBackend())
	buildResult.Touch()
	buildState := buildResult.GetState()

	// Check repo task state
	repoSignature := tasks.Signature{
		Name: "repo",
		UUID: UUID,
	}
	repoResult := result.NewAsyncResult(&repoSignature, server.GetBackend())
	repoResult.Touch()
	repoState := repoResult.GetState()

	// Determine overall pipeline state
	var pipelineState string
	if buildState.State == "FAILURE" {
		pipelineState = "FAILED"
	} else if buildState.State == "SUCCESS" && repoState.State == "SUCCESS" {
		pipelineState = "DONE"
	} else if buildState.State == "SUCCESS" && repoState.State == "FAILURE" {
		pipelineState = "FAILED"
	} else if buildState.State == "SUCCESS" && (repoState.State == "PENDING" || repoState.State == "RECEIVED" || repoState.State == "STARTED") {
		pipelineState = "REPO"
	} else {
		pipelineState = buildState.State
	}

	res := fmt.Sprintf("{ \"pipelineId\": \"%s\", \"state\": \"%s\" }", UUID, pipelineState)
	fmt.Fprintf(w, res)
}

func RetryHandler(w http.ResponseWriter, r *http.Request) {
	keys, ok := r.URL.Query()["uuid"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error": "uuid parameter is required"}`)
		return
	}
	taskUUID := keys[0]

	// Check if monitoring is enabled
	if !irgshConfig.Monitoring.Enabled || monitoringRegistry == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"error": "monitoring is not enabled, retry requires job tracking"}`)
		return
	}

	// Get job info from monitoring registry
	job, err := monitoringRegistry.GetJob(taskUUID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, `{"error": "job not found: %s"}`, taskUUID)
		return
	}

	// Build the submission object from job info
	submission := Submission{
		TaskUUID:       job.TaskUUID,
		Timestamp:      time.Now(),
		PackageName:    job.PackageName,
		PackageVersion: job.PackageVersion,
		PackageURL:     job.PackageURL,
		SourceURL:      job.SourceURL,
		Maintainer:     job.Maintainer,
		Component:      job.Component,
		IsExperimental: job.IsExperimental,
		PackageBranch:  job.PackageBranch,
		SourceBranch:   job.SourceBranch,
	}

	jsonStr, err := json.Marshal(submission)
	if err != nil {
		log.Println(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to marshal submission"}`)
		return
	}

	buildSignature := tasks.Signature{
		Name: "build",
		UUID: submission.TaskUUID,
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: string(jsonStr),
			},
		},
	}

	repoSignature := tasks.Signature{
		Name: "repo",
		UUID: submission.TaskUUID,
	}

	chain, _ := tasks.NewChain(&buildSignature, &repoSignature)
	_, err = server.SendChain(chain)
	if err != nil {
		log.Println("Could not send chain : " + err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to queue retry task: %s"}`, err.Error())
		return
	}

	// Update job state to PENDING
	if err := monitoringRegistry.UpdateJobState(taskUUID, "PENDING"); err != nil {
		log.Printf("Failed to update job state: %v\n", err)
	}

	log.Printf("Job %s retried successfully\n", taskUUID)

	payload := SubmitPayloadResponse{PipelineId: submission.TaskUUID}
	jsonStr, _ = json.Marshal(payload)
	fmt.Fprintf(w, string(jsonStr))
}

func artifactUploadHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		keys, ok := r.URL.Query()["id"]

		if !ok || len(keys[0]) < 1 {
			log.Println("Url Param 'uuid' is missing")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		id := keys[0]

		targetPath := irgshConfig.Chief.Workdir + "/artifacts"
		err = os.MkdirAll(targetPath, 0755)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// parse and validate file and post parameters
		file, _, err := r.FormFile("uploadFile")
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// check file type, detectcontenttype only needs the first 512 bytes
		filetype := http.DetectContentType(fileBytes)
		switch filetype {
		case "application/gzip", "application/x-gzip":
			break
		default:
			log.Println("File upload rejected: should be a compressed tar.gz file.")
			w.WriteHeader(http.StatusBadRequest)
		}

		fileName := id + ".tar.gz"
		newPath := filepath.Join(targetPath, fileName)

		// write file
		newFile, err := os.Create(newPath)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer newFile.Close()
		if _, err := newFile.Write(fileBytes); err != nil || newFile.Close() != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// TODO should be in JSON string
		w.WriteHeader(http.StatusOK)
	})
}

func logUploadHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		keys, ok := r.URL.Query()["id"]

		if !ok || len(keys[0]) < 1 {
			log.Println("Url Param 'id' is missing")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		id := keys[0]

		keys, ok = r.URL.Query()["type"]

		if !ok || len(keys[0]) < 1 {
			log.Println("Url Param 'type' is missing")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		logType := keys[0]

		targetPath := irgshConfig.Chief.Workdir + "/logs"
		err = os.MkdirAll(targetPath, 0755)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// parse and validate file and post parameters
		file, _, err := r.FormFile("uploadFile")
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// check file type, detectcontenttype only needs the first 512 bytes
		filetype := strings.Split(http.DetectContentType(fileBytes), ";")[0]
		switch filetype {
		case "text/plain":
			break
		default:
			log.Println("File upload rejected: should be a plain text log file.")
			w.WriteHeader(http.StatusBadRequest)
		}

		fileName := id + "." + logType + ".log"
		newPath := filepath.Join(targetPath, fileName)

		// write file
		newFile, err := os.Create(newPath)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer newFile.Close()
		if _, err := newFile.Write(fileBytes); err != nil || newFile.Close() != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// TODO should be in JSON string
		w.WriteHeader(http.StatusOK)
	})
}

func BuildISOHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("iso")
	signature := tasks.Signature{
		Name: "iso",
		UUID: uuid.New().String(),
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: "iso-specific-value",
			},
		},
	}
	// TODO grab the asyncResult here
	_, err := server.SendTask(&signature)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Println("Could not send task : " + err.Error())
		fmt.Fprintf(w, "500")
	}
	// TODO should be in JSON string
	w.WriteHeader(http.StatusOK)
}

func submissionUploadHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error

		targetPath := irgshConfig.Chief.Workdir + "/submissions"
		err = os.MkdirAll(targetPath, 0755)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Check for auth token first
		file, _, err := r.FormFile("token")
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		fileBytes, err := ioutil.ReadAll(file)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// write file
		id := uuid.New().String()
		fileName := id + ".token"
		newPath := filepath.Join(targetPath, fileName)
		newFile, err := os.Create(newPath)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer newFile.Close()
		if _, err := newFile.Write(fileBytes); err != nil || newFile.Close() != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		gnupgDir := "GNUPGHOME=" + irgshConfig.Chief.GnupgDir
		if irgshConfig.IsDev {
			gnupgDir = ""
		}

		cmdStr := "cd " + targetPath + " && "
		cmdStr += gnupgDir + " gpg --verify " + newPath
		err = exec.Command("bash", "-c", cmdStr).Run()
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, "401 Unauthorized")
			return
		}

		// parse and validate file and post parameters
		file, _, err = r.FormFile("blob")
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		fileBytes, err = ioutil.ReadAll(file)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// check file type, detectcontenttype only needs the first 512 bytes
		filetype := strings.Split(http.DetectContentType(fileBytes), ";")[0]
		log.Println(filetype)
		if !strings.Contains(filetype, "gzip") {
			log.Println("File upload rejected: should be a tar.gz file.")
			w.WriteHeader(http.StatusBadRequest)
		}
		fileName = id + ".tar.gz"
		newPath = filepath.Join(targetPath, fileName)

		// write file
		newFile, err = os.Create(newPath)
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer newFile.Close()
		if _, err := newFile.Write(fileBytes); err != nil || newFile.Close() != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "{\"id\":\""+id+"\"}")
	})
}

func MaintainersHandler(w http.ResponseWriter, r *http.Request) {
	gnupgDir := "GNUPGHOME=" + irgshConfig.Chief.GnupgDir
	if irgshConfig.IsDev {
		gnupgDir = ""
	}

	cmdStr := gnupgDir + " gpg --list-key | tail -n +2"

	output, err := exec.Command("bash", "-c", cmdStr).Output()
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "500")
		return
	}
	fmt.Fprintf(w, string(output))
}

func VersionHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "{\"version\":\""+app.Version+"\"}")
}

// formatDuration formats a duration into human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
}
