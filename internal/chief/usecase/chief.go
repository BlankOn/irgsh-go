package usecase

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/RichardKnop/machinery/v1"
	"github.com/RichardKnop/machinery/v1/backends/result"
	"github.com/RichardKnop/machinery/v1/tasks"
	"github.com/google/uuid"

	chiefrepository "github.com/blankon/irgsh-go/internal/chief/repository"
	"github.com/blankon/irgsh-go/internal/config"
	"github.com/blankon/irgsh-go/internal/monitoring"
	"github.com/blankon/irgsh-go/internal/storage"
	"github.com/blankon/irgsh-go/pkg/httputil"
	"github.com/blankon/irgsh-go/pkg/systemutil"
)

// maxLogSize is the maximum size of a log file upload (10 MB).
const maxLogSize = 10 << 20

// safeIDPattern matches strings that contain only safe characters for use in
// file paths and identifiers: alphanumeric, dots, hyphens, underscores.
var safeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._+-]+$`)


type ChiefUsecase struct {
	config             config.IrgshConfig
	server             *machinery.Server
	monitoringRegistry *monitoring.Registry
	storage            *chiefrepository.Storage
	gpg                *chiefrepository.GPG
	version            string
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
		config:             cfg,
		server:             server,
		monitoringRegistry: registry,
		storage:            storage,
		gpg:                gpg,
		version:            version,
	}
}

// GetVersion returns the version string for use by handlers.
func (s *ChiefUsecase) GetVersion() string {
	return s.version
}

func (s *ChiefUsecase) GetMaintainers() []Maintainer {
	output, err := s.gpg.ListKeysWithColons()
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

			if len(fields) > 4 && len(fields[4]) >= 16 {
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
	out := `<!DOCTYPE html>
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
        @keyframes spin-gear {
            from { transform: rotate(0deg); }
            to { transform: rotate(360deg); }
        }
        .spinning-gear {
            vertical-align: middle;
            animation: spin-gear 2s linear infinite;
        }
    </style>
</head>
<body>
    <div class="header">
        <div>irgsh-chief ` + s.version + `</div>
    </div>
`

	out += `<div class="section-title">Package Maintainers</div>`

	maintainers := s.GetMaintainers()
	if len(maintainers) > 0 {
		out += `
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
			out += fmt.Sprintf(`
				<tr>
					<td style="font-family: monospace;">%s</td>
					<td>%s</td>
					<td>%s</td>
				</tr>`,
				html.EscapeString(m.KeyID),
				html.EscapeString(m.Name),
				html.EscapeString(m.Email),
			)
		}

		out += `
			</tbody>
		</table>`
	} else {
		out += `<div class="empty-state">No maintainers found</div>`
	}

	if s.config.Monitoring.Enabled && s.monitoringRegistry != nil {
		instances, err := s.monitoringRegistry.ListInstances("", "")
		if err != nil {
			log.Printf("Failed to list instances: %v\n", err)
		} else {
			summary, err := s.monitoringRegistry.GetSummary()
			if err != nil {
				log.Printf("Failed to get summary: %v\n", err)
			}

			out += `<div class="section-title">Workers</div>`

			out += fmt.Sprintf(`
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
				out += fmt.Sprintf(`
        <div class="summary-item">
            <div class="summary-number" style="color: #2196F3;">%d</div>
            <div>%s</div>
        </div>
`, count, html.EscapeString(typeName))
			}

			out += `
    </div>
`

			if len(instances) > 0 {
				out += `
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

					out += fmt.Sprintf(`
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
						html.EscapeString(string(instance.InstanceType)),
						html.EscapeString(instance.Hostname),
						statusClass,
						html.EscapeString(string(instance.Status)),
						uptimeStr,
						instance.ActiveTasks,
						instance.Concurrency,
						cpuStr,
						memStr,
						diskStr,
					)
				}

				out += `
        </tbody>
    </table>
`
			} else {
				out += `<div class="empty-state">No worker instances found</div>`
			}
		}

		jobs, err := s.monitoringRegistry.GetRecentJobs(50)
		if err != nil {
			log.Printf("Failed to list jobs: %v\n", err)
		} else if len(jobs) > 0 {
			out += `<div class="section-title">Recent Packaging Jobs</div>`
			out += `
			<div style="margin-bottom: 10px;">
				<label for="statusFilter" style="margin-right: 5px;">Filter by status:</label>
				<select id="statusFilter" onchange="filterJobsByStatus(this.value)" style="padding: 4px 8px; font-size: 0.95em;">
					<option value="all">All</option>
					<option value="DONE">DONE</option>
					<option value="FAILED">FAILED</option>
					<option value="PENDING">PENDING</option>
					<option value="UNKNOWN">UNKNOWN</option>
				</select>
			</div>
			<table id="packagingJobsTable">
				<thead>
					<tr>
						<th>Timestamp</th>
						<th>Package</th>
						<th>Version</th>
						<th>Maintainer</th>
						<th>Component</th>
						<th>Build</th>
						<th>Repo</th>
						<th>Status</th>
						<th>UUID</th>
					</tr>
				</thead>
				<tbody>`

			jakartaLoc, locErr := time.LoadLocation("Asia/Jakarta")
			if locErr != nil {
				jakartaLoc = time.UTC
			}

			stageClass := func(state string) string {
				switch state {
				case "SUCCESS":
					return "status-online"
				case "FAILURE":
					return "status-offline"
				case "STARTED", "RECEIVED":
					return "status-warning"
				default:
					return ""
				}
			}

			for _, job := range jobs {
				// Skip machinery query for terminal states -- trust SQLite
				if storage.IsTerminalState(job.State) || job.State == "UNKNOWN" {
					// Use stored build/repo states as-is
				} else {
					buildState, repoState, currentStage := monitoring.GetJobStagesFromMachinery(
						s.server.GetBackend(),
						job.TaskUUID,
					)

					// If machinery returns empty for both, data has expired
					if buildState == "" && repoState == "" {
						// Keep current SQLite state
					} else {
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
							overallState = "PENDING"
						} else if buildState != "" {
							overallState = "PENDING"
						} else {
							overallState = "PENDING"
						}

						job.State = overallState

						// Persist terminal states to SQLite so they survive Redis TTL expiry
						if storage.IsTerminalState(overallState) {
							s.monitoringRegistry.UpdateJobStages(job.TaskUUID, buildState, repoState, currentStage)
							s.monitoringRegistry.UpdateJobState(job.TaskUUID, overallState)
						}
					}
				}

				statusClass := ""
				statusText := job.State
				filterStatus := job.State
				switch job.State {
				case "DONE":
					statusClass = "status-online"
				case "FAILED":
					statusClass = "status-offline"
					if job.BuildState == "FAILURE" {
						statusText = "FAILED (build)"
					} else if job.RepoState == "FAILURE" {
						statusText = "FAILED (repo)"
					}
				case "PENDING":
					if time.Since(job.SubmittedAt) > 24*time.Hour {
						statusClass = "status-offline"
						statusText = "STALLED"
					} else {
						statusText = `<svg class="spinning-gear" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#ff9800" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>`
					}
					filterStatus = "PENDING"
				case "UNKNOWN":
					statusClass = "status-offline"
					statusText = "UNKNOWN"
				default:
					statusText = `<svg class="spinning-gear" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#ff9800" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>`
					filterStatus = "PENDING"
				}

				jakartaTime := job.SubmittedAt.In(jakartaLoc)
				timeStr := fmt.Sprintf("%s<br><span style=\"color: #666; font-size: 0.9em;\">(%s)</span>",
					jakartaTime.Format("2006-01-02 15:04:05 MST"),
					formatRelativeTime(job.SubmittedAt))

				expTag := ""
				if job.IsExperimental {
					expTag = " <span style=\"color: #ff9800; font-weight: bold;\">[experimental]</span>"
				}

				packageCell := html.EscapeString(job.PackageName) + expTag
				var repoLinks []string
				if job.SourceURL != "" {
					branchText := job.SourceBranch
					if branchText == "" {
						branchText = "default"
					}
					linkURL := job.SourceURL + "/tree/" + branchText
					repoLinks = append(repoLinks, fmt.Sprintf(`<a href="%s" target="_blank">source (%s)</a>`, html.EscapeString(linkURL), html.EscapeString(branchText)))
				}
				if job.PackageURL != "" {
					branchText := job.PackageBranch
					if branchText == "" {
						branchText = "default"
					}
					linkURL := job.PackageURL + "/tree/" + branchText
					repoLinks = append(repoLinks, fmt.Sprintf(`<a href="%s" target="_blank">package (%s)</a>`, html.EscapeString(linkURL), html.EscapeString(branchText)))
				}
				if len(repoLinks) > 0 {
					packageCell += fmt.Sprintf(`<br><span style="font-size: 0.85em; color: #666;">%s</span>`,
						strings.Join(repoLinks, ", "))
				}

				buildStateText := job.BuildState
				if buildStateText == "" {
					buildStateText = "-"
				}
				repoStateText := job.RepoState
				if repoStateText == "" {
					repoStateText = "-"
				}

				out += fmt.Sprintf(`
					<tr data-status="%s">
						<td>%s</td>
						<td>%s</td>
						<td>%s</td>
						<td>%s</td>
						<td>%s</td>
						<td><span class="%s">%s</span><br><a href="/logs/%s.build.log" target="_blank" style="font-size:0.85em;">log</a></td>
						<td><span class="%s">%s</span><br><a href="/logs/%s.repo.log" target="_blank" style="font-size:0.85em;">log</a></td>
						<td><span class="%s">%s</span></td>
						<td style="font-family: monospace; font-size: 0.85em;">%s</td>
					</tr>`,
					html.EscapeString(filterStatus),
					timeStr,
					packageCell,
					html.EscapeString(job.PackageVersion),
					html.EscapeString(job.Maintainer),
					html.EscapeString(job.Component),
					stageClass(job.BuildState),
					html.EscapeString(buildStateText),
					html.EscapeString(job.TaskUUID),
					stageClass(job.RepoState),
					html.EscapeString(repoStateText),
					html.EscapeString(job.TaskUUID),
					statusClass,
					statusText,
					html.EscapeString(job.TaskUUID),
				)
			}

			out += `
				</tbody>
			</table>
			`
		}

		// ISO jobs section
		isoJobs, isoErr := s.monitoringRegistry.GetRecentISOJobs(50)
		if isoErr != nil {
			log.Printf("Failed to list ISO jobs: %v\n", isoErr)
		} else if len(isoJobs) > 0 {
			out += `<div class="section-title">Recent ISO Build Jobs</div>`
			out += `
			<table>
				<thead>
					<tr>
						<th>Timestamp</th>
						<th>Repository</th>
						<th>Branch</th>
						<th>Status</th>
						<th>UUID</th>
					</tr>
				</thead>
				<tbody>`

			jakartaLoc, locErr := time.LoadLocation("Asia/Jakarta")
			if locErr != nil {
				jakartaLoc = time.UTC
			}

			for _, isoJob := range isoJobs {
				isoStatusClass := ""
				switch isoJob.State {
				case "SUCCESS", "DONE":
					isoStatusClass = "status-online"
				case "FAILURE", "FAILED":
					isoStatusClass = "status-offline"
				case "STARTED", "RECEIVED":
					isoStatusClass = "status-warning"
				}

				isoTime := isoJob.SubmittedAt.In(jakartaLoc)
				isoTimeStr := fmt.Sprintf("%s<br><span style=\"color: #666; font-size: 0.9em;\">(%s)</span>",
					isoTime.Format("2006-01-02 15:04:05 MST"),
					formatRelativeTime(isoJob.SubmittedAt))

				out += fmt.Sprintf(`
					<tr>
						<td>%s</td>
						<td>%s</td>
						<td>%s</td>
						<td><span class="%s">%s</span></td>
						<td style="font-family: monospace; font-size: 0.85em;">%s</td>
					</tr>`,
					isoTimeStr,
					html.EscapeString(isoJob.RepoURL),
					html.EscapeString(isoJob.Branch),
					isoStatusClass,
					html.EscapeString(isoJob.State),
					html.EscapeString(isoJob.TaskUUID),
				)
			}

			out += `
				</tbody>
			</table>
			`
		}
	}

	out += `
    <div class="refresh-info">
        Page auto-refreshes every 10 seconds
    </div>
    <script>
    function filterJobsByStatus(status) {
        var table = document.getElementById('packagingJobsTable');
        if (!table) return;
        var rows = table.getElementsByTagName('tr');
        for (var i = 1; i < rows.length; i++) {
            var row = rows[i];
            if (status === 'all' || row.getAttribute('data-status') === status) {
                row.style.display = '';
            } else {
                row.style.display = 'none';
            }
        }
    }
    </script>
</body>
</html>
`

	return out, nil
}

func (s *ChiefUsecase) SubmitPackage(submission Submission) (SubmitPayloadResponse, error) {
	if !safeIDPattern.MatchString(submission.MaintainerFingerprint) {
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "invalid maintainer fingerprint")
	}
	if !safeIDPattern.MatchString(submission.PackageName) {
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "invalid package name")
	}
	if !safeIDPattern.MatchString(submission.Tarball) {
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "invalid tarball identifier")
	}

	submission.Timestamp = time.Now()
	submission.TaskUUID = submission.Timestamp.Format("2006-01-02-150405") + "_" + uuid.New().String() + "_" + submission.MaintainerFingerprint + "_" + submission.PackageName

	if err := s.storage.EnsureDir(filepath.Join(s.storage.SubmissionsDir(), submission.TaskUUID)); err != nil {
		log.Println(err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	src := filepath.Join(s.storage.SubmissionsDir(), submission.Tarball+".tar.gz")
	path := s.storage.SubmissionTarballPath(submission.TaskUUID)
	if err := systemutil.MoveFile(src, path); err != nil {
		log.Println(err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	if err := s.storage.ExtractSubmission(submission.TaskUUID); err != nil {
		log.Println(err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	src = filepath.Join(s.storage.SubmissionsDir(), submission.Tarball+".token")
	path = s.storage.SubmissionSignaturePath(submission.TaskUUID)
	if err := systemutil.MoveFile(src, path); err != nil {
		log.Println(err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	if err := s.gpg.VerifySignedSubmission(s.storage.SubmissionDirPath(submission.TaskUUID)); err != nil {
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

	chain, err := tasks.NewChain(&buildSignature, &repoSignature)
	if err != nil {
		log.Printf("Could not create chain: %v\n", err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}
	if _, err = s.server.SendChain(chain); err != nil {
		log.Printf("Could not send chain: %v\n", err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	if s.config.Monitoring.Enabled && s.monitoringRegistry != nil {
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
		if err := s.monitoringRegistry.RecordJob(job); err != nil {
			log.Printf("Failed to record job: %v\n", err)
		}
	}

	return SubmitPayloadResponse{PipelineID: submission.TaskUUID}, nil
}

func (s *ChiefUsecase) BuildStatus(UUID string) (BuildStatusResponse, error) {
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
	buildResult := result.NewAsyncResult(&buildSignature, s.server.GetBackend())
	buildResult.Touch()
	buildState := buildResult.GetState()

	repoSignature := tasks.Signature{
		Name: "repo",
		UUID: UUID,
	}
	repoResult := result.NewAsyncResult(&repoSignature, s.server.GetBackend())
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
		pipelineState = "BUILDING"
	} else if buildState.State == "" {
		pipelineState = "UNKNOWN"
	} else {
		pipelineState = "BUILDING"
	}

	return BuildStatusResponse{
		PipelineID:  UUID,
		JobStatus:   pipelineState,
		BuildStatus: buildState.State,
		RepoStatus:  repoState.State,
		State:       pipelineState,
	}, nil
}

func (s *ChiefUsecase) ISOStatus(UUID string) (string, string, error) {
	isoSignature := tasks.Signature{
		Name: "iso",
		UUID: UUID,
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: "xyz",
			},
		},
	}
	isoResult := result.NewAsyncResult(&isoSignature, s.server.GetBackend())
	isoResult.Touch()
	isoState := isoResult.GetState()

	isoStatusStr := isoState.State

	var jobStatus string
	if isoStatusStr == "FAILURE" {
		jobStatus = "FAILED"
	} else if isoStatusStr == "SUCCESS" {
		jobStatus = "DONE"
	} else if isoStatusStr == "PENDING" || isoStatusStr == "RECEIVED" || isoStatusStr == "STARTED" {
		jobStatus = "BUILDING"
	} else if isoStatusStr == "" {
		jobStatus = "UNKNOWN"
	} else {
		jobStatus = isoStatusStr
	}

	return jobStatus, isoStatusStr, nil
}

func (s *ChiefUsecase) RetryPipeline(oldTaskUUID string) (SubmitPayloadResponse, error) {
	if !safeIDPattern.MatchString(oldTaskUUID) {
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "invalid pipeline identifier")
	}
	if !s.config.Monitoring.Enabled || s.monitoringRegistry == nil {
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusServiceUnavailable, `{"error": "monitoring is not enabled, retry requires job tracking"}`)
	}

	job, err := s.monitoringRegistry.GetJob(oldTaskUUID)
	if err != nil {
		log.Printf("Job not found for retry: %s: %v\n", oldTaskUUID, err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusNotFound, `{"error": "job not found"}`)
	}

	parts := strings.Split(oldTaskUUID, "_")
	var maintainerFingerprint string
	if len(parts) >= 3 {
		maintainerFingerprint = parts[2]
	}

	newTimestamp := time.Now()
	newTaskUUID := newTimestamp.Format("2006-01-02-150405") + "_" + uuid.New().String() + "_" + maintainerFingerprint + "_" + job.PackageName

	submissionsDir := s.storage.SubmissionsDir()
	oldTarball := filepath.Join(submissionsDir, oldTaskUUID+".tar.gz")
	newTarball := filepath.Join(submissionsDir, newTaskUUID+".tar.gz")
	oldDir := filepath.Join(submissionsDir, oldTaskUUID)
	newDir := filepath.Join(submissionsDir, newTaskUUID)

	log.Printf("Retry: copying submission files from %s to %s\n", oldTaskUUID, newTaskUUID)

	if _, err := os.Stat(oldTarball); os.IsNotExist(err) {
		log.Printf("Original submission tarball not found: %s\n", oldTarball)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusNotFound, `{"error": "original submission tarball not found, cannot retry"}`)
	}

	if err := s.storage.CopyFileWithSudo(oldTarball, newTarball); err != nil {
		log.Printf("Failed to copy submission tarball: %v\n", err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, `{"error": "failed to copy submission files for retry"}`)
	}

	if _, err := os.Stat(oldDir); err == nil {
		if err := s.storage.CopyDirWithSudo(oldDir, newDir); err != nil {
			log.Printf("Failed to copy submission directory: %v\n", err)
			return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, `{"error": "failed to copy submission directory for retry"}`)
		}
	}

	if err := s.storage.ChownWithSudo(newTarball); err != nil {
		log.Printf("Failed to chown tarball: %v\n", err)
	}

	if err := s.storage.ChownRecursiveWithSudo(newDir); err != nil {
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

	chain, err := tasks.NewChain(&buildSignature, &repoSignature)
	if err != nil {
		log.Printf("Could not create chain: %v\n", err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, `{"error": "failed to create retry task chain"}`)
	}
	_, err = s.server.SendChain(chain)
	if err != nil {
		log.Println("Could not send chain : " + err.Error())
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, `{"error": "failed to queue retry task"}`)
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
	if err := s.monitoringRegistry.RecordJob(newJob); err != nil {
		log.Printf("Failed to record retry job: %v\n", err)
	}

	log.Printf("Job %s retried as new pipeline %s\n", oldTaskUUID, newTaskUUID)

	return SubmitPayloadResponse{PipelineID: newTaskUUID}, nil
}

func (s *ChiefUsecase) UploadArtifact(id string, file io.Reader) error {
	if !safeIDPattern.MatchString(id) {
		return httputil.NewHTTPError(http.StatusBadRequest, "invalid artifact id")
	}

	targetPath := s.storage.ArtifactsDir()
	if err := s.storage.EnsureDir(targetPath); err != nil {
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
		os.Remove(newPath)
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	if _, err := io.Copy(newFile, file); err != nil {
		log.Println(err.Error())
		os.Remove(newPath)
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	return nil
}

func (s *ChiefUsecase) UploadLog(id string, logType string, file io.Reader) error {
	if !safeIDPattern.MatchString(id) {
		return httputil.NewHTTPError(http.StatusBadRequest, "invalid log id")
	}
	if !safeIDPattern.MatchString(logType) {
		return httputil.NewHTTPError(http.StatusBadRequest, "invalid log type")
	}

	targetPath := s.storage.LogsDir()
	if err := s.storage.EnsureDir(targetPath); err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	fileBytes, err := io.ReadAll(io.LimitReader(file, maxLogSize))
	if err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusBadRequest, "")
	}

	filetype := strings.Split(http.DetectContentType(fileBytes), ";")[0]
	if filetype != "text/plain" {
		log.Println("File upload rejected: should be a plain text log file.")
		return httputil.NewHTTPError(http.StatusBadRequest, "")
	}

	fileName := id + "." + logType + ".log"
	newPath := filepath.Join(targetPath, fileName)

	newFile, err := os.Create(newPath)
	if err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}
	defer newFile.Close()

	if _, err := newFile.Write(fileBytes); err != nil {
		log.Println(err.Error())
		return httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	return nil
}

func (s *ChiefUsecase) BuildISO(submission ISOSubmission) (SubmitPayloadResponse, error) {
	if submission.RepoURL == "" {
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "repoUrl is required")
	}
	if submission.Branch == "" {
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "branch is required")
	}

	submission.Timestamp = time.Now()
	submission.TaskUUID = submission.Timestamp.Format("2006-01-02-150405") + "_" + uuid.New().String() + "_iso"

	jsonStr, err := json.Marshal(submission)
	if err != nil {
		log.Println(err.Error())
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "400")
	}

	signature := tasks.Signature{
		Name: "iso",
		UUID: submission.TaskUUID,
		Args: []tasks.Arg{
			{
				Type:  "string",
				Value: string(jsonStr),
			},
		},
	}
	if _, err := s.server.SendTask(&signature); err != nil {
		log.Printf("Could not send ISO task: %v\n", err)
		return SubmitPayloadResponse{}, httputil.NewHTTPError(http.StatusInternalServerError, "500")
	}

	if s.config.Monitoring.Enabled && s.monitoringRegistry != nil {
		isoJob := monitoring.ISOJobInfo{
			TaskUUID:    submission.TaskUUID,
			RepoURL:     submission.RepoURL,
			Branch:      submission.Branch,
			SubmittedAt: submission.Timestamp,
			State:       "PENDING",
		}
		if err := s.monitoringRegistry.RecordISOJob(isoJob); err != nil {
			log.Printf("Failed to record ISO job: %v\n", err)
		}
	}

	return SubmitPayloadResponse{PipelineID: submission.TaskUUID}, nil
}

func (s *ChiefUsecase) UploadSubmission(tokenData []byte, blob io.Reader) (string, error) {
	targetPath := s.storage.SubmissionsDir()
	if err := s.storage.EnsureDir(targetPath); err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	id := uuid.New().String()

	// Write token file
	tokenPath := filepath.Join(targetPath, id+".token")
	tokenFile, err := os.Create(tokenPath)
	if err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}
	if _, err := tokenFile.Write(tokenData); err != nil {
		tokenFile.Close()
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}
	if err := tokenFile.Close(); err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	if err := s.gpg.VerifyFile(tokenPath); err != nil {
		log.Println(err)
		os.Remove(tokenPath)
		return "", httputil.NewHTTPError(http.StatusUnauthorized, "401 Unauthorized")
	}

	// Write blob file with content-type validation
	blobPath := filepath.Join(targetPath, id+".tar.gz")
	blobFile, err := os.Create(blobPath)
	if err != nil {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}
	defer blobFile.Close()

	header := make([]byte, 512)
	n, err := blob.Read(header)
	if err != nil && err != io.EOF {
		log.Println(err.Error())
		return "", httputil.NewHTTPError(http.StatusBadRequest, "")
	}
	header = header[:n]

	filetype := http.DetectContentType(header)
	switch filetype {
	case "application/gzip", "application/x-gzip":
	default:
		log.Println("File upload rejected: should be a tar.gz file.")
		os.Remove(blobPath)
		os.Remove(tokenPath)
		return "", httputil.NewHTTPError(http.StatusBadRequest, "")
	}

	if _, err := blobFile.Write(header); err != nil {
		log.Println(err.Error())
		os.Remove(blobPath)
		os.Remove(tokenPath)
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	if _, err := io.Copy(blobFile, blob); err != nil {
		log.Println(err.Error())
		os.Remove(blobPath)
		os.Remove(tokenPath)
		return "", httputil.NewHTTPError(http.StatusInternalServerError, "")
	}

	return id, nil
}

func (s *ChiefUsecase) ListMaintainersRaw() (string, error) {
	output, err := s.gpg.ListKeys()
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
