package usecase

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/blankon/irgsh-go/internal/monitoring"
	"github.com/blankon/irgsh-go/internal/storage"
)

//go:embed templates/dashboard.html
var dashboardTmplStr string

//go:embed templates/logviewer.html
var logViewerTmplStr string

// The dashboard carries the same top bar as the rest of the BlankOn services,
// wordmark included. Chief serves no static directory, so the asset is
// embedded in the binary and handed out by its own route.
//
//go:embed assets/logo.png
var logoPNG []byte

// LogoPNG returns the wordmark shown in the top bar.
func (d *DashboardService) LogoPNG() []byte {
	return logoPNG
}

// View models for the dashboard template.

type DashboardData struct {
	Version       string
	Maintainers   []domain.Maintainer
	HasMonitoring bool
	Summary       SummaryView
	Workers       []WorkerView
	Jobs          []JobView
	ISOJobs       []ISOJobView
	ImportJobs    []ImportJobView
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
	Dist        string
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
	FilterStatus    string
	TimeFormatted   string
	TimeRelative    string
	DistComponent   string
	PackageName     string
	PackageVersion  string
	Maintainer      string
	IsExperimental  bool
	RepoLinks       []RepoLink
	BuildStageClass string
	BuildStateText  string
	RepoStageClass  string
	RepoStateText   string
	StatusClass     string
	StatusText      string
	ShowSpinner     bool
	TaskUUID        string
}

type ISOJobView struct {
	TimeFormatted string
	TimeRelative  string
	Dist          string
	Branch        string
	State         string
	StatusClass   string
	TaskUUID      string
}

type ImportJobView struct {
	TimeFormatted  string
	TimeRelative   string
	SourceURL      string
	DistComponent  string
	Packages       string
	Maintainer     string
	IsExperimental bool
	State          string
	StatusClass    string
	ShowSpinner    bool
	TaskUUID       string
}

// DashboardService renders the chief dashboard HTML.
type DashboardService struct {
	version       string
	taskQueue     TaskQueue
	maintainerSvc *MaintainerService
	registry      InstanceRegistry
	jobStore      JobStore
	isoStore      ISOJobStore
	importStore   ImportJobStore
	tmpl          *template.Template
	logViewerTmpl *template.Template
}

// LogViewerData is the view model of the streaming log page.
type LogViewerData struct {
	TaskUUID string
	LogType  string
}

func NewDashboardService(
	version string,
	taskQueue TaskQueue,
	maintainerSvc *MaintainerService,
	registry InstanceRegistry,
	jobStore JobStore,
	isoStore ISOJobStore,
	importStore ImportJobStore,
) (*DashboardService, error) {
	tmpl, err := template.New("dashboard").Parse(dashboardTmplStr)
	if err != nil {
		return nil, fmt.Errorf("parse dashboard template: %w", err)
	}
	logViewerTmpl, err := template.New("logviewer").Parse(logViewerTmplStr)
	if err != nil {
		return nil, fmt.Errorf("parse log viewer template: %w", err)
	}
	return &DashboardService{
		version:       version,
		taskQueue:     taskQueue,
		maintainerSvc: maintainerSvc,
		registry:      registry,
		jobStore:      jobStore,
		isoStore:      isoStore,
		importStore:   importStore,
		tmpl:          tmpl,
		logViewerTmpl: logViewerTmpl,
	}, nil
}

func (d *DashboardService) RenderIndexHTML(w io.Writer) error {
	data := d.buildDashboardData()
	return d.tmpl.Execute(w, data)
}

// RenderLogViewerHTML renders the page that streams one job log.
func (d *DashboardService) RenderLogViewerHTML(w io.Writer, taskUUID, logType string) error {
	return d.logViewerTmpl.Execute(w, LogViewerData{TaskUUID: taskUUID, LogType: logType})
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
	data.ImportJobs = d.buildImportJobViews()

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
			Dist:        inst.Dist,
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
		DistComponent:   joinDistComponent(job.Dist, job.Component),
		PackageName:     job.PackageName,
		PackageVersion:  job.PackageVersion,
		Maintainer:      job.Maintainer,
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
		job.State = d.resolveTaskState("iso", job.TaskUUID, job.State, d.isoStore.UpdateISOJobState)
		statusClass := jobStateClass(job.State)

		jakartaTime := job.SubmittedAt.In(jakartaLoc)
		views = append(views, ISOJobView{
			TimeFormatted: jakartaTime.Format("2006-01-02 15:04:05 MST"),
			TimeRelative:  formatRelativeTime(job.SubmittedAt),
			Dist:          job.Dist,
			Branch:        job.Branch,
			State:         job.State,
			StatusClass:   statusClass,
			TaskUUID:      job.TaskUUID,
		})
	}
	return views
}

func (d *DashboardService) buildImportJobViews() []ImportJobView {
	if d.importStore == nil {
		return nil
	}
	importJobs, err := d.importStore.GetRecentImportJobs(50)
	if err != nil {
		log.Printf("Failed to list import jobs: %v\n", err)
		return nil
	}
	if len(importJobs) == 0 {
		return nil
	}

	jakartaLoc, locErr := time.LoadLocation("Asia/Jakarta")
	if locErr != nil {
		jakartaLoc = time.UTC
	}

	views := make([]ImportJobView, 0, len(importJobs))
	for _, job := range importJobs {
		job.State = d.resolveTaskState("import", job.TaskUUID, job.State, d.importStore.UpdateImportJobState)
		jakartaTime := job.SubmittedAt.In(jakartaLoc)
		views = append(views, ImportJobView{
			TimeFormatted:  jakartaTime.Format("2006-01-02 15:04:05 MST"),
			TimeRelative:   formatRelativeTime(job.SubmittedAt),
			SourceURL:      job.SourceURL,
			DistComponent:  joinDistComponent(job.Dist, job.Component),
			Packages:       formatPackageList(job.Packages),
			Maintainer:     job.Maintainer,
			IsExperimental: job.IsExperimental,
			State:          job.State,
			StatusClass:    jobStateClass(job.State),
			ShowSpinner:    isJobRunning(job.State),
			TaskUUID:       job.TaskUUID,
		})
	}
	return views
}

// resolveTaskState brings a stored job state up to date from the task queue.
//
// The store only ever holds the state the job was recorded with, so without
// this a single-task job (ISO, import) is displayed as PENDING forever, even
// after the worker has finished or failed it.
func (d *DashboardService) resolveTaskState(taskName, taskUUID, stored string, persist func(string, string) error) string {
	if d.taskQueue == nil || storage.IsTerminalState(stored) || stored == "UNKNOWN" {
		return stored
	}

	state := d.taskQueue.GetTaskState(taskName, taskUUID)
	// Machinery expires task results; keep what we recorded.
	if state == "" || state == stored {
		return stored
	}

	if persist != nil {
		if err := persist(taskUUID, state); err != nil {
			log.Printf("Failed to update %s job state: %v\n", taskName, err)
		}
	}
	return state
}

// formatPackageList renders a stored package list as "a, b, c", including
// rows written before the list was stored comma separated.
func formatPackageList(packages string) string {
	names := strings.FieldsFunc(packages, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	return strings.Join(names, ", ")
}

// isJobRunning reports whether a single-task job is still in flight, so the
// dashboard can show the same spinner the packaging jobs use.
func isJobRunning(state string) bool {
	switch state {
	case "SUCCESS", "DONE", "FAILURE", "FAILED", "UNKNOWN", "":
		return false
	}
	return true
}

// jobStateClass maps a job state to the dashboard's status CSS class.
func jobStateClass(state string) string {
	switch state {
	case "SUCCESS", "DONE":
		return "status-online"
	case "FAILURE", "FAILED":
		return "status-offline"
	case "STARTED", "RECEIVED":
		return "status-warning"
	}
	return ""
}

// joinDistComponent renders the distribution and component a job targets as
// one "dist/component" cell. Rows written before either was recorded still
// show whichever half they have, rather than a stray slash.
func joinDistComponent(dist, component string) string {
	switch {
	case dist == "":
		return component
	case component == "":
		return dist
	default:
		return dist + "/" + component
	}
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
