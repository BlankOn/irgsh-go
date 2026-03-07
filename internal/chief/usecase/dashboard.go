package usecase

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"log"
	"sort"
	"time"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/blankon/irgsh-go/internal/monitoring"
	"github.com/blankon/irgsh-go/internal/storage"
)

//go:embed templates/dashboard.html
var dashboardTmplStr string

// View models for the dashboard template.

type DashboardData struct {
	Version       string
	Maintainers   []domain.Maintainer
	HasMonitoring bool
	Summary       SummaryView
	Workers       []WorkerView
	Jobs          []JobView
	ISOJobs       []ISOJobView
}

type SummaryView struct {
	Total   int
	Online  int
	Offline int
	ByType  []TypeCount
}

type TypeCount struct {
	Name  string
	Count int
}

type WorkerView struct {
	Type        string
	BadgeClass  string
	Hostname    string
	Status      string
	StatusClass string
	Uptime      string
	ActiveTasks int
	Concurrency int
	CPU         string
	Memory      string
	Disk        string
}

type RepoLink struct {
	URL   string
	Label string
}

type JobView struct {
	FilterStatus   string
	TimeFormatted  string
	TimeRelative   string
	PackageName    string
	PackageVersion string
	Maintainer     string
	Component      string
	IsExperimental bool
	RepoLinks      []RepoLink
	BuildStageClass string
	BuildStateText  string
	RepoStageClass  string
	RepoStateText   string
	StatusClass    string
	StatusText     string
	ShowSpinner    bool
	TaskUUID       string
}

type ISOJobView struct {
	TimeFormatted string
	TimeRelative  string
	RepoURL       string
	Branch        string
	State         string
	StatusClass   string
	TaskUUID      string
}

// DashboardService renders the chief dashboard HTML.
type DashboardService struct {
	version       string
	taskQueue     TaskQueue
	maintainerSvc *MaintainerService
	registry      InstanceRegistry
	jobStore      JobStore
	isoStore      ISOJobStore
	tmpl          *template.Template
}

func NewDashboardService(
	version string,
	taskQueue TaskQueue,
	maintainerSvc *MaintainerService,
	registry InstanceRegistry,
	jobStore JobStore,
	isoStore ISOJobStore,
) (*DashboardService, error) {
	tmpl, err := template.New("dashboard").Parse(dashboardTmplStr)
	if err != nil {
		return nil, fmt.Errorf("parse dashboard template: %w", err)
	}
	return &DashboardService{
		version:       version,
		taskQueue:     taskQueue,
		maintainerSvc: maintainerSvc,
		registry:      registry,
		jobStore:      jobStore,
		isoStore:      isoStore,
		tmpl:          tmpl,
	}, nil
}

func (d *DashboardService) RenderIndexHTML(w io.Writer) error {
	data := d.buildDashboardData()
	return d.tmpl.Execute(w, data)
}

func (d *DashboardService) buildDashboardData() DashboardData {
	data := DashboardData{
		Version:     d.version,
		Maintainers: d.maintainerSvc.GetMaintainers(),
	}

	if d.registry == nil {
		return data
	}
	data.HasMonitoring = true

	instances, err := d.registry.ListInstances("", "")
	if err != nil {
		log.Printf("Failed to list instances: %v\n", err)
	} else {
		summary, err := d.registry.GetSummary()
		if err != nil {
			log.Printf("Failed to get summary: %v\n", err)
		}
		data.Summary = buildSummaryView(summary)
		data.Workers = buildWorkerViews(instances)
	}
	data.Jobs = d.buildJobViews()
	data.ISOJobs = d.buildISOJobViews()

	return data
}

func buildSummaryView(s monitoring.InstanceSummary) SummaryView {
	sv := SummaryView{
		Total:   s.Total,
		Online:  s.Online,
		Offline: s.Offline,
	}
	for name, count := range s.ByType {
		sv.ByType = append(sv.ByType, TypeCount{Name: name, Count: count})
	}
	sort.Slice(sv.ByType, func(i, j int) bool {
		return sv.ByType[i].Name < sv.ByType[j].Name
	})
	return sv
}

func buildWorkerViews(instances []*monitoring.InstanceInfo) []WorkerView {
	views := make([]WorkerView, 0, len(instances))
	for _, inst := range instances {
		badgeClass := "badge-builder"
		switch inst.InstanceType {
		case monitoring.InstanceTypeRepo:
			badgeClass = "badge-repo"
		case monitoring.InstanceTypeISO:
			badgeClass = "badge-iso"
		}

		statusClass := "status-offline"
		if inst.Status == monitoring.StatusOnline {
			statusClass = "status-online"
		}

		memStr := monitoring.FormatBytes(inst.MemoryUsage)
		if inst.MemoryTotal > 0 {
			memStr += " / " + monitoring.FormatBytes(inst.MemoryTotal)
		}

		diskStr := monitoring.FormatBytes(inst.DiskUsage)
		if inst.DiskTotal > 0 {
			diskStr += " / " + monitoring.FormatBytes(inst.DiskTotal)
		}

		views = append(views, WorkerView{
			Type:        string(inst.InstanceType),
			BadgeClass:  badgeClass,
			Hostname:    inst.Hostname,
			Status:      string(inst.Status),
			StatusClass: statusClass,
			Uptime:      formatDuration(time.Since(inst.StartTime)),
			ActiveTasks: inst.ActiveTasks,
			Concurrency: inst.Concurrency,
			CPU:         fmt.Sprintf("%.1f", inst.CPUUsage),
			Memory:      memStr,
			Disk:        diskStr,
		})
	}
	return views
}

func (d *DashboardService) buildJobViews() []JobView {
	if d.jobStore == nil {
		return nil
	}
	jobs, err := d.jobStore.GetRecentJobs(50)
	if err != nil {
		log.Printf("Failed to list jobs: %v\n", err)
		return nil
	}
	if len(jobs) == 0 {
		return nil
	}

	d.resolveJobStates(jobs)

	jakartaLoc, locErr := time.LoadLocation("Asia/Jakarta")
	if locErr != nil {
		jakartaLoc = time.UTC
	}

	views := make([]JobView, 0, len(jobs))
	for _, job := range jobs {
		views = append(views, buildJobView(job, jakartaLoc))
	}
	return views
}

func (d *DashboardService) resolveJobStates(jobs []*storage.JobInfo) {
	for _, job := range jobs {
		if storage.IsTerminalState(job.State) || job.State == "UNKNOWN" {
			continue
		}

		buildState := d.taskQueue.GetTaskState("build", job.TaskUUID)
		repoState := d.taskQueue.GetTaskState("repo", job.TaskUUID)

		// If machinery returns empty for both, data has expired
		if buildState == "" && repoState == "" {
			continue
		}

		currentStage := domain.DeriveCurrentStage(buildState, repoState)
		job.BuildState = buildState
		job.RepoState = repoState
		job.CurrentStage = currentStage

		var overallState string
		switch {
		case buildState == "FAILURE":
			overallState = "FAILED"
		case buildState == "SUCCESS" && repoState == "SUCCESS":
			overallState = "DONE"
		case buildState == "SUCCESS" && repoState == "FAILURE":
			overallState = "FAILED"
		default:
			overallState = "PENDING"
		}

		job.State = overallState

		if storage.IsTerminalState(overallState) {
			d.jobStore.UpdateJobStages(job.TaskUUID, buildState, repoState, currentStage)
			d.jobStore.UpdateJobState(job.TaskUUID, overallState)
		}
	}
}

func buildJobView(job *storage.JobInfo, loc *time.Location) JobView {
	statusClass := ""
	statusText := job.State
	filterStatus := job.State
	showSpinner := false

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
			showSpinner = true
		}
		filterStatus = "PENDING"
	case "UNKNOWN":
		statusClass = "status-offline"
		statusText = "UNKNOWN"
	default:
		showSpinner = true
		filterStatus = "PENDING"
	}

	buildStateText := job.BuildState
	if buildStateText == "" {
		buildStateText = "-"
	}
	repoStateText := job.RepoState
	if repoStateText == "" {
		repoStateText = "-"
	}

	var repoLinks []RepoLink
	if job.SourceURL != "" {
		branchText := job.SourceBranch
		if branchText == "" {
			branchText = "default"
		}
		repoLinks = append(repoLinks, RepoLink{
			URL:   job.SourceURL + "/tree/" + branchText,
			Label: "source (" + branchText + ")",
		})
	}
	if job.PackageURL != "" {
		branchText := job.PackageBranch
		if branchText == "" {
			branchText = "default"
		}
		repoLinks = append(repoLinks, RepoLink{
			URL:   job.PackageURL + "/tree/" + branchText,
			Label: "package (" + branchText + ")",
		})
	}

	jakartaTime := job.SubmittedAt.In(loc)

	return JobView{
		FilterStatus:    filterStatus,
		TimeFormatted:   jakartaTime.Format("2006-01-02 15:04:05 MST"),
		TimeRelative:    formatRelativeTime(job.SubmittedAt),
		PackageName:     job.PackageName,
		PackageVersion:  job.PackageVersion,
		Maintainer:      job.Maintainer,
		Component:       job.Component,
		IsExperimental:  job.IsExperimental,
		RepoLinks:       repoLinks,
		BuildStageClass: stageClass(job.BuildState),
		BuildStateText:  buildStateText,
		RepoStageClass:  stageClass(job.RepoState),
		RepoStateText:   repoStateText,
		StatusClass:     statusClass,
		StatusText:      statusText,
		ShowSpinner:     showSpinner,
		TaskUUID:        job.TaskUUID,
	}
}

func (d *DashboardService) buildISOJobViews() []ISOJobView {
	if d.isoStore == nil {
		return nil
	}
	isoJobs, err := d.isoStore.GetRecentISOJobs(50)
	if err != nil {
		log.Printf("Failed to list ISO jobs: %v\n", err)
		return nil
	}
	if len(isoJobs) == 0 {
		return nil
	}

	jakartaLoc, locErr := time.LoadLocation("Asia/Jakarta")
	if locErr != nil {
		jakartaLoc = time.UTC
	}

	views := make([]ISOJobView, 0, len(isoJobs))
	for _, job := range isoJobs {
		statusClass := ""
		switch job.State {
		case "SUCCESS", "DONE":
			statusClass = "status-online"
		case "FAILURE", "FAILED":
			statusClass = "status-offline"
		case "STARTED", "RECEIVED":
			statusClass = "status-warning"
		}

		jakartaTime := job.SubmittedAt.In(jakartaLoc)
		views = append(views, ISOJobView{
			TimeFormatted: jakartaTime.Format("2006-01-02 15:04:05 MST"),
			TimeRelative:  formatRelativeTime(job.SubmittedAt),
			RepoURL:       job.RepoURL,
			Branch:        job.Branch,
			State:         job.State,
			StatusClass:   statusClass,
			TaskUUID:      job.TaskUUID,
		})
	}
	return views
}

func stageClass(state string) string {
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
