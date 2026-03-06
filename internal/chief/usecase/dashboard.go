package usecase

import (
	"fmt"
	"html"
	"log"
	"strings"
	"time"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/blankon/irgsh-go/internal/monitoring"
	"github.com/blankon/irgsh-go/internal/storage"
)

// DashboardService renders the chief dashboard HTML.
type DashboardService struct {
	version       string
	taskQueue     TaskQueue
	maintainerSvc *MaintainerService
	registry      InstanceRegistry
	jobStore      JobStore
	isoStore      ISOJobStore
}

func NewDashboardService(
	version string,
	taskQueue TaskQueue,
	maintainerSvc *MaintainerService,
	registry InstanceRegistry,
	jobStore JobStore,
	isoStore ISOJobStore,
) *DashboardService {
	return &DashboardService{
		version:       version,
		taskQueue:     taskQueue,
		maintainerSvc: maintainerSvc,
		registry:      registry,
		jobStore:      jobStore,
		isoStore:      isoStore,
	}
}

func (d *DashboardService) RenderIndexHTML() (string, error) {
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
        <div>irgsh-chief ` + html.EscapeString(d.version) + `</div>
    </div>
`

	out += `<div class="section-title">Package Maintainers</div>`

	maintainers := d.maintainerSvc.GetMaintainers()
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

	if d.registry != nil {
		instances, err := d.registry.ListInstances("", "")
		if err != nil {
			log.Printf("Failed to list instances: %v\n", err)
		} else {
			summary, err := d.registry.GetSummary()
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

		d.renderPackagingJobs(&out)
		d.renderISOJobs(&out)
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

func (d *DashboardService) renderPackagingJobs(out *string) {
	if d.jobStore == nil {
		return
	}
	jobs, err := d.jobStore.GetRecentJobs(50)
	if err != nil {
		log.Printf("Failed to list jobs: %v\n", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	*out += `<div class="section-title">Recent Packaging Jobs</div>`
	*out += `
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
			buildState := d.taskQueue.GetTaskState("build", job.TaskUUID)
			repoState := d.taskQueue.GetTaskState("repo", job.TaskUUID)
			currentStage := domain.DeriveCurrentStage(buildState, repoState)

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
					d.jobStore.UpdateJobStages(job.TaskUUID, buildState, repoState, currentStage)
					d.jobStore.UpdateJobState(job.TaskUUID, overallState)
				}
			}
		}

		statusClass := ""
		statusText := html.EscapeString(job.State)
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

		*out += fmt.Sprintf(`
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

	*out += `
				</tbody>
			</table>
			`
}

func (d *DashboardService) renderISOJobs(out *string) {
	if d.isoStore == nil {
		return
	}
	isoJobs, isoErr := d.isoStore.GetRecentISOJobs(50)
	if isoErr != nil {
		log.Printf("Failed to list ISO jobs: %v\n", isoErr)
		return
	}
	if len(isoJobs) == 0 {
		return
	}

	*out += `<div class="section-title">Recent ISO Build Jobs</div>`
	*out += `
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

		*out += fmt.Sprintf(`
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

	*out += `
				</tbody>
			</table>
			`
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
