package usecase

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RichardKnop/machinery/v1"
	"github.com/RichardKnop/machinery/v1/backends/result"
	"github.com/RichardKnop/machinery/v1/tasks"
	"github.com/google/uuid"

	chiefrepository "github.com/blankon/irgsh-go/internal/chief/repository"
	"github.com/blankon/irgsh-go/internal/config"
	"github.com/blankon/irgsh-go/internal/monitoring"
	"github.com/blankon/irgsh-go/pkg/httputil"
)

type ChiefUsecase struct {
	Config             config.IrgshConfig
	Server             *machinery.Server
	MonitoringRegistry *monitoring.Registry
	Storage            *chiefrepository.Storage
	GPG                *chiefrepository.GPG
	Version            string
}

func NewChiefUsecase(
	cfg config.IrgshConfig,
	server *machinery.Server,
	registry *monitoring.Registry,
	storage *chiefrepository.Storage,
	gpg *chiefrepository.GPG,
	version string,
) *ChiefUsecase {
	return &ChiefUsecase{
		Config:             cfg,
		Server:             server,
		MonitoringRegistry: registry,
		Storage:            storage,
		GPG:                gpg,
		Version:            version,
	}
}

func (s *ChiefUsecase) GetMaintainers() []Maintainer {
	output, err := s.GPG.ListKeysWithColons()
	if err != nil {
		log.Printf("Failed to list GPG keys: %v\n", err)
		return []Maintainer{}
	}
	return parseGPGKeys(output)
}

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
		case "pub":
			if currentKey != nil {
				maintainers = append(maintainers, *currentKey)
			}
			currentKey = &Maintainer{
				KeyID: "",
				Name:  "",
				Email: "",
			}

			if len(fields) > 4 && len(fields[4]) >= 8 {
				currentKey.KeyID = fields[4][len(fields[4])-16:]
			}

		case "uid":
			if currentKey != nil && len(fields) > 9 {
				uid := fields[9]

				if strings.Contains(uid, "<") && strings.Contains(uid, ">") {
					parts := strings.SplitN(uid, "<", 2)
					currentKey.Name = strings.TrimSpace(parts[0])
					if len(parts) > 1 {
						emailPart := strings.SplitN(parts[1], ">", 2)
						currentKey.Email = strings.TrimSpace(emailPart[0])
					}
				} else {
					currentKey.Name = uid
				}
			}
		}
	}

	if currentKey != nil {
		maintainers = append(maintainers, *currentKey)
	}

	return maintainers
}

func (s *ChiefUsecase) RenderIndexHTML() (string, error) {
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
        <div>irgsh-chief ` + s.Version + `</div>
    </div>
`

	html += `<div class="section-title">Package Maintainers</div>`

	maintainers := s.GetMaintainers()
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

	if s.Config.Monitoring.Enabled && s.MonitoringRegistry != nil {
		instances, err := s.MonitoringRegistry.ListInstances("", "")
		if err != nil {
			log.Printf("Failed to list instances: %v\n", err)
		} else {
			summary, err := s.MonitoringRegistry.GetSummary()
			if err != nil {
				log.Printf("Failed to get summary: %v\n", err)
			}

			html += `<div class="section-title">Workers</div>`

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

			if len(instances) > 0 {
				html += `
    <table>
        <thead>
            <tr>
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
					badgeClass := "badge badge-builder"
					switch instance.InstanceType {
					case monitoring.InstanceTypeRepo:
						badgeClass = "badge badge-repo"
					case monitoring.InstanceTypeISO:
						badgeClass = "badge badge-iso"
					}

					statusClass := "status-offline"
					if instance.Status == monitoring.StatusOnline {
						statusClass = "status-online"
					}

					uptime := time.Since(instance.StartTime)
					uptimeStr := formatDuration(uptime)

					cpuStr := fmt.Sprintf("%.1f", instance.CPUUsage)

					memStr := monitoring.FormatBytes(instance.MemoryUsage)
					if instance.MemoryTotal > 0 {
						memStr += " / " + monitoring.FormatBytes(instance.MemoryTotal)
					}

					diskStr := monitoring.FormatBytes(instance.DiskUsage)
					if instance.DiskTotal > 0 {
						diskStr += " / " + monitoring.FormatBytes(instance.DiskTotal)
					}

					html += fmt.Sprintf(`
            <tr>
                <td><span class="%s">%s</span></td>
                <td>%s</td>
                <td><span class="%s">%s</span></td>
                <td>%s</td>
                <td>%d / %d</td>
                <td class="metric">%s / 100</td>
                <td class="metric">%s</td>
                <td class="metric">%s</td>
            </tr>`,
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

		jobs, err := s.MonitoringRegistry.GetRecentJobs(50)
		if err != nil {
			log.Printf("Failed to list jobs: %v\n", err)
		} else if len(jobs) > 0 {
			html += `<div class="section-title">Recent Packaging Jobs</div>`
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

			for _, job := range jobs {
				buildState, repoState, currentStage := monitoring.GetJobStagesFromMachinery(
					s.Server.GetBackend(),
					job.TaskUUID,
				)

				job.BuildState = buildState
				job.RepoState = repoState
				job.CurrentStage = currentStage

				var overallState string
				if buildState == "FAILURE" {
					overallState = "FAILED"
				} else if buildState == "SUCCESS" && repoState == "SUCCESS" {
					overallState = "DONE"
				} else if buildState == "SUCCESS" && repoState == "FAILURE" {
					overallState = "FAILED"
				} else if buildState == "SUCCESS" && (repoState == "PENDING" || repoState == "RECEIVED" || repoState == "STARTED") {
					overallState = "REPO"
				} else if buildState != "" {
					overallState = buildState
				} else {
					overallState = "PENDING"
				}

				job.State = overallState

				statusClass := ""
				statusText := overallState
				switch overallState {
				case "DONE":
					statusClass = "status-online"
				case "FAILED":
					statusClass = "status-offline"
					if buildState == "FAILURE" {
						statusText = "FAILED (build)"
					} else if repoState == "FAILURE" {
						statusText = "FAILED (repo)"
					}
				case "REPO":
					statusClass = "status-warning"
					statusText = "REPO"
				case "STARTED":
					statusClass = "status-warning"
					statusText = "STARTED (" + currentStage + ")"
				case "PENDING":
					if time.Since(job.SubmittedAt) > 24*time.Hour {
						statusClass = "status-offline"
						statusText = "STALLED"
					} else {
						statusText = "PENDING"
					}
				default:
					statusText = "PENDING"
				}

				jakartaLoc, _ := time.LoadLocation("Asia/Jakarta")
				jakartaTime := job.SubmittedAt.In(jakartaLoc)
				timeStr := fmt.Sprintf("%s<br><span style=\"color: #666; font-size: 0.9em;\">(%s)</span>",
					jakartaTime.Format("2006-01-02 15:04:05 MST"),
					formatRelativeTime(job.SubmittedAt))

				expTag := ""
				if job.IsExperimental {
					expTag = " <span style=\"color: #ff9800; font-weight: bold;\">[experimental]</span>"
				}

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

	return html, nil
}

func (s *ChiefUsecase) SubmitPackage(submission Submission) (SubmitPayloadResponse, error) {
	submission.Timestamp = time.Now()
	submission.TaskUUID = submission.Timestamp.Format("2006-01-02-150405") + "_" + uuid.New().String() + "_" + submission.MaintainerFingerprint + "_" + submission.PackageName

	if err := s.Storage.EnsureDir(filepath.Join(s.Storage.SubmissionsDir(), submission.TaskUUID)); err != nil {
		log.Println(err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	src := filepath.Join(s.Storage.SubmissionsDir(), submission.Tarball+".tar.gz")
	path := s.Storage.SubmissionTarballPath(submission.TaskUUID)
	if err := s.Storage.MoveFile(src, path); err != nil {
		log.Println(err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	if err := s.Storage.ExtractSubmission(submission.TaskUUID); err != nil {
		log.Println(err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	src = filepath.Join(s.Storage.SubmissionsDir(), submission.Tarball+".token")
	path = s.Storage.SubmissionSignaturePath(submission.TaskUUID)
	if err := s.Storage.MoveFile(src, path); err != nil {
		log.Println(err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	if err := s.GPG.VerifySignedSubmission(s.Storage.SubmissionDirPath(submission.TaskUUID)); err != nil {
		log.Println(err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusUnauthorized, "401 Unauthorized")
	}

	jsonStr, err := json.Marshal(submission)
	if err != nil {
		fmt.Println(err.Error())
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "400")
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
	_, err = s.Server.SendChain(chain)
	if err != nil {
		fmt.Println("Could not send chain : " + err.Error())
	}

	if s.Config.Monitoring.Enabled && s.MonitoringRegistry != nil {
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
		if err := s.MonitoringRegistry.RecordJob(job); err != nil {
			log.Printf("Failed to record job: %v\n", err)
		}
	}

	return SubmitPayloadResponse{PipelineId: submission.TaskUUID}, nil
}

func (s *ChiefUsecase) BuildStatus(UUID string) (string, error) {
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
	buildResult := result.NewAsyncResult(&buildSignature, s.Server.GetBackend())
	buildResult.Touch()
	buildState := buildResult.GetState()

	repoSignature := tasks.Signature{
		Name: "repo",
		UUID: UUID,
	}
	repoResult := result.NewAsyncResult(&repoSignature, s.Server.GetBackend())
	repoResult.Touch()
	repoState := repoResult.GetState()

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

	return pipelineState, nil
}

func (s *ChiefUsecase) RetryPipeline(oldTaskUUID string) (SubmitPayloadResponse, error) {
	if !s.Config.Monitoring.Enabled || s.MonitoringRegistry == nil {
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusServiceUnavailable, `{"error": "monitoring is not enabled, retry requires job tracking"}`)
	}

	job, err := s.MonitoringRegistry.GetJob(oldTaskUUID)
	if err != nil {
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusNotFound, fmt.Sprintf(`{"error": "job not found: %s"}`, oldTaskUUID))
	}

	parts := strings.Split(oldTaskUUID, "_")
	var maintainerFingerprint string
	if len(parts) >= 3 {
		maintainerFingerprint = parts[2]
	}

	newTimestamp := time.Now()
	newTaskUUID := newTimestamp.Format("2006-01-02-150405") + "_" + uuid.New().String() + "_" + maintainerFingerprint + "_" + job.PackageName

	submissionsDir := s.Storage.SubmissionsDir()
	oldTarball := filepath.Join(submissionsDir, oldTaskUUID+".tar.gz")
	newTarball := filepath.Join(submissionsDir, newTaskUUID+".tar.gz")
	oldDir := filepath.Join(submissionsDir, oldTaskUUID)
	newDir := filepath.Join(submissionsDir, newTaskUUID)

	log.Printf("Retry: copying submission files from %s to %s\n", oldTaskUUID, newTaskUUID)

	if _, err := os.Stat(oldTarball); os.IsNotExist(err) {
		log.Printf("Original submission tarball not found: %s\n", oldTarball)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusNotFound, `{"error": "original submission tarball not found, cannot retry"}`)
	}

	if err := s.Storage.CopyFileWithSudo(oldTarball, newTarball); err != nil {
		log.Printf("Failed to copy submission tarball: %v\n", err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf(`{"error": "failed to copy submission files for retry: %s"}`, err.Error()))
	}

	if _, err := os.Stat(oldDir); err == nil {
		if err := s.Storage.CopyDirWithSudo(oldDir, newDir); err != nil {
			log.Printf("Failed to copy submission directory: %v\n", err)
			return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf(`{"error": "failed to copy submission directory for retry: %s"}`, err.Error()))
		}
	}

	if err := s.Storage.ChownWithSudo(newTarball); err != nil {
		log.Printf("Failed to chown tarball: %v\n", err)
	}

	if err := s.Storage.ChownRecursiveWithSudo(newDir); err != nil {
		log.Printf("Failed to chown submission directory: %v\n", err)
	}

	log.Printf("Retry: submission files copied successfully\n")

	submission := Submission{
		TaskUUID:              newTaskUUID,
		Timestamp:             newTimestamp,
		PackageName:           job.PackageName,
		PackageVersion:        job.PackageVersion,
		PackageURL:            job.PackageURL,
		SourceURL:             job.SourceURL,
		Maintainer:            job.Maintainer,
		MaintainerFingerprint: maintainerFingerprint,
		Component:             job.Component,
		IsExperimental:        job.IsExperimental,
		PackageBranch:         job.PackageBranch,
		SourceBranch:          job.SourceBranch,
	}

	jsonStr, err := json.Marshal(submission)
	if err != nil {
		log.Println(err.Error())
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, `{"error": "failed to marshal submission"}`)
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
	_, err = s.Server.SendChain(chain)
	if err != nil {
		log.Println("Could not send chain : " + err.Error())
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf(`{"error": "failed to queue retry task: %s"}`, err.Error()))
	}

	newJob := monitoring.JobInfo{
		TaskUUID:       newTaskUUID,
		PackageName:    job.PackageName,
		PackageVersion: job.PackageVersion,
		Maintainer:     job.Maintainer,
		Component:      job.Component,
		IsExperimental: job.IsExperimental,
		SubmittedAt:    newTimestamp,
		State:          "PENDING",
		PackageURL:     job.PackageURL,
		SourceURL:      job.SourceURL,
		PackageBranch:  job.PackageBranch,
		SourceBranch:   job.SourceBranch,
	}
	if err := s.MonitoringRegistry.RecordJob(newJob); err != nil {
		log.Printf("Failed to record retry job: %v\n", err)
	}

	log.Printf("Job %s retried as new pipeline %s\n", oldTaskUUID, newTaskUUID)

	return SubmitPayloadResponse{PipelineId: newTaskUUID}, nil
}

func (s *ChiefUsecase) UploadArtifact(id string, file io.Reader) error {
	targetPath := s.Storage.ArtifactsDir()
	if err := s.Storage.EnsureDir(targetPath); err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	fileName := id + ".tar.gz"
	newPath := filepath.Join(targetPath, fileName)

	newFile, err := os.Create(newPath)
	if err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}
	defer newFile.Close()

	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && err != io.EOF {
		log.Println(err.Error())
		os.Remove(newPath)
		return httputil.NewHTTPError(http.StatusBadRequest, "")
	}
	header = header[:n]

	filetype := http.DetectContentType(header)
	switch filetype {
	case "application/gzip", "application/x-gzip":
	default:
		log.Println("File upload rejected: should be a compressed tar.gz file.")
		os.Remove(newPath)
		return httputil.NewHTTPError(http.StatusBadRequest, "")
	}

	if _, err := newFile.Write(header); err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	if _, err := io.Copy(newFile, file); err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	return nil
}

func (s *ChiefUsecase) UploadLog(id string, logType string, file io.Reader) error {
	targetPath := s.Storage.LogsDir()
	if err := s.Storage.EnsureDir(targetPath); err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusBadRequest, "")
	}

	filetype := strings.Split(http.DetectContentType(fileBytes), ";")[0]
	invalidType := false
	switch filetype {
	case "text/plain":
	default:
		log.Println("File upload rejected: should be a plain text log file.")
		invalidType = true
	}

	fileName := id + "." + logType + ".log"
	newPath := filepath.Join(targetPath, fileName)

	newFile, err := os.Create(newPath)
	if err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}
	defer newFile.Close()

	if _, err := newFile.Write(fileBytes); err != nil || newFile.Close() != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	if invalidType {
		return httputil.NewHTTPError(http.StatusBadRequest, "")
	}

	return nil
}

func (s *ChiefUsecase) BuildISO() error {
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
	if _, err := s.Server.SendTask(&signature); err != nil {
		return httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}
	return nil
}

func (s *ChiefUsecase) UploadSubmission(r *http.Request) (string, error) {
	targetPath := s.Storage.SubmissionsDir()
	if err := s.Storage.EnsureDir(targetPath); err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	if err := r.ParseMultipartForm(512 << 20); err != nil {
		log.Printf("ParseMultipartForm error: %v", err)
		return "", httputil.NewHTTPError(http.StatusBadRequest, "")
	}

	file, _, err := r.FormFile("token")
	if err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusBadRequest, "")
	}
	defer file.Close()

	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusBadRequest, "")
	}

	id := uuid.New().String()
	fileName := id + ".token"
	newPath := filepath.Join(targetPath, fileName)
	newFile, err := os.Create(newPath)
	if err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}
	defer newFile.Close()
	if _, err := newFile.Write(fileBytes); err != nil || newFile.Close() != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	if err := s.GPG.VerifyFile(newPath); err != nil {
		log.Println(err)
		return "", httputil.NewHTTPError(http.StatusUnauthorized, "401 Unauthorized")
	}

	file, _, err = r.FormFile("blob")
	if err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusBadRequest, "")
	}
	defer file.Close()

	fileName = id + ".tar.gz"
	newPath = filepath.Join(targetPath, fileName)
	newFile, err = os.Create(newPath)
	if err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}
	defer newFile.Close()

	header := make([]byte, 512)
	n, err := file.Read(header)
	if err != nil && err != io.EOF {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusBadRequest, "")
	}
	header = header[:n]

	filetype := strings.Split(http.DetectContentType(header), ";")[0]
	log.Println(filetype)
	if !strings.Contains(filetype, "gzip") {
		log.Println("File upload rejected: should be a tar.gz file.")
		os.Remove(newPath)
		return "", httputil.NewHTTPError(http.StatusBadRequest, "")
	}

	if _, err := newFile.Write(header); err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	if _, err := io.Copy(newFile, file); err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	return id, nil
}

func (s *ChiefUsecase) ListMaintainersRaw() (string, error) {
	output, err := s.GPG.ListKeys()
	if err != nil {
		log.Println(err)
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}
	return output, nil
}

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

func formatRelativeTime(t time.Time) string {
	d := time.Since(t)
	if d < 0 {
		return "just now"
	}
	seconds := int(d.Seconds())
	if seconds < 60 {
		if seconds == 1 {
			return "1 second ago"
		}
		return fmt.Sprintf("%d seconds ago", seconds)
	}
	minutes := int(d.Minutes())
	if minutes < 60 {
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	}
	hours := int(d.Hours())
	if hours < 24 {
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := hours / 24
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}
