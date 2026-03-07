package usecase

import (
	"bytes"
	"testing"
	"time"

	"github.com/blankon/irgsh-go/internal/monitoring"
	"github.com/blankon/irgsh-go/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSummaryView(t *testing.T) {
	summary := monitoring.InstanceSummary{
		Total:   5,
		Online:  3,
		Offline: 2,
		ByType:  map[string]int{"builder": 3, "repo": 1, "iso": 1},
	}
	sv := buildSummaryView(summary)
	assert.Equal(t, 5, sv.Total)
	assert.Equal(t, 3, sv.Online)
	assert.Equal(t, 2, sv.Offline)
	assert.Len(t, sv.ByType, 3)
	// sorted alphabetically
	assert.Equal(t, "builder", sv.ByType[0].Name)
	assert.Equal(t, "iso", sv.ByType[1].Name)
	assert.Equal(t, "repo", sv.ByType[2].Name)
}

func TestBuildSummaryView_NilByType(t *testing.T) {
	summary := monitoring.InstanceSummary{Total: 0, Online: 0, Offline: 0}
	sv := buildSummaryView(summary)
	assert.Empty(t, sv.ByType)
}

func TestBuildWorkerViews(t *testing.T) {
	now := time.Now()
	instances := []*monitoring.InstanceInfo{
		{
			InstanceType: monitoring.InstanceTypeBuilder,
			Hostname:     "builder-1",
			Status:       monitoring.StatusOnline,
			StartTime:    now.Add(-2 * time.Hour),
			ActiveTasks:  1,
			Concurrency:  4,
			CPUUsage:     55.5,
			MemoryUsage:  1024 * 1024 * 512,
			MemoryTotal:  1024 * 1024 * 1024 * 4,
			DiskUsage:    1024 * 1024 * 1024 * 10,
			DiskTotal:    1024 * 1024 * 1024 * 100,
		},
		{
			InstanceType: monitoring.InstanceTypeRepo,
			Hostname:     "repo-1",
			Status:       monitoring.StatusOffline,
			StartTime:    now.Add(-30 * time.Minute),
		},
		{
			InstanceType: monitoring.InstanceTypeISO,
			Hostname:     "iso-1",
			Status:       monitoring.StatusOnline,
			StartTime:    now.Add(-5 * time.Minute),
		},
	}

	views := buildWorkerViews(instances)
	require.Len(t, views, 3)

	// builder
	assert.Equal(t, "badge-builder", views[0].BadgeClass)
	assert.Equal(t, "builder-1", views[0].Hostname)
	assert.Equal(t, "status-online", views[0].StatusClass)
	assert.Equal(t, "55.5", views[0].CPU)
	assert.Equal(t, 1, views[0].ActiveTasks)
	assert.Equal(t, 4, views[0].Concurrency)

	// repo
	assert.Equal(t, "badge-repo", views[1].BadgeClass)
	assert.Equal(t, "status-offline", views[1].StatusClass)

	// iso
	assert.Equal(t, "badge-iso", views[2].BadgeClass)
	assert.Equal(t, "status-online", views[2].StatusClass)
}

func TestBuildWorkerViews_Empty(t *testing.T) {
	views := buildWorkerViews(nil)
	assert.Empty(t, views)
}

func TestBuildJobView(t *testing.T) {
	loc := time.UTC
	now := time.Now()

	t.Run("done job", func(t *testing.T) {
		job := &storage.JobInfo{
			TaskUUID:       "test-uuid",
			PackageName:    "pkg",
			PackageVersion: "1.0",
			Maintainer:     "User",
			State:          "DONE",
			BuildState:     "SUCCESS",
			RepoState:      "SUCCESS",
			SubmittedAt:    now.Add(-5 * time.Minute),
			SourceURL:      "https://git.example.com/src",
			SourceBranch:   "main",
			PackageURL:     "https://git.example.com/pkg",
			PackageBranch:  "master",
		}
		v := buildJobView(job, loc)
		assert.Equal(t, "DONE", v.FilterStatus)
		assert.Equal(t, "status-online", v.StatusClass)
		assert.Equal(t, "DONE", v.StatusText)
		assert.False(t, v.ShowSpinner)
		assert.Equal(t, "SUCCESS", v.BuildStateText)
		assert.Equal(t, "SUCCESS", v.RepoStateText)
		assert.Len(t, v.RepoLinks, 2)
		assert.Contains(t, v.RepoLinks[0].URL, "/tree/main")
		assert.Contains(t, v.RepoLinks[1].URL, "/tree/master")
	})

	t.Run("failed build", func(t *testing.T) {
		job := &storage.JobInfo{
			State:       "FAILED",
			BuildState:  "FAILURE",
			SubmittedAt: now,
		}
		v := buildJobView(job, loc)
		assert.Equal(t, "status-offline", v.StatusClass)
		assert.Equal(t, "FAILED (build)", v.StatusText)
	})

	t.Run("failed repo", func(t *testing.T) {
		job := &storage.JobInfo{
			State:       "FAILED",
			BuildState:  "SUCCESS",
			RepoState:   "FAILURE",
			SubmittedAt: now,
		}
		v := buildJobView(job, loc)
		assert.Equal(t, "FAILED (repo)", v.StatusText)
	})

	t.Run("pending job shows spinner", func(t *testing.T) {
		job := &storage.JobInfo{
			State:       "PENDING",
			SubmittedAt: now.Add(-1 * time.Minute),
		}
		v := buildJobView(job, loc)
		assert.True(t, v.ShowSpinner)
		assert.Equal(t, "PENDING", v.FilterStatus)
	})

	t.Run("stalled pending job", func(t *testing.T) {
		job := &storage.JobInfo{
			State:       "PENDING",
			SubmittedAt: now.Add(-48 * time.Hour),
		}
		v := buildJobView(job, loc)
		assert.False(t, v.ShowSpinner)
		assert.Equal(t, "STALLED", v.StatusText)
		assert.Equal(t, "status-offline", v.StatusClass)
	})

	t.Run("unknown state", func(t *testing.T) {
		job := &storage.JobInfo{
			State:       "UNKNOWN",
			SubmittedAt: now,
		}
		v := buildJobView(job, loc)
		assert.Equal(t, "status-offline", v.StatusClass)
		assert.Equal(t, "UNKNOWN", v.StatusText)
	})

	t.Run("empty build/repo state shows dash", func(t *testing.T) {
		job := &storage.JobInfo{
			State:       "PENDING",
			SubmittedAt: now,
		}
		v := buildJobView(job, loc)
		assert.Equal(t, "-", v.BuildStateText)
		assert.Equal(t, "-", v.RepoStateText)
	})

	t.Run("default branch label when branch is empty", func(t *testing.T) {
		job := &storage.JobInfo{
			State:       "DONE",
			SubmittedAt: now,
			SourceURL:   "https://git.example.com/src",
			PackageURL:  "https://git.example.com/pkg",
		}
		v := buildJobView(job, loc)
		assert.Len(t, v.RepoLinks, 2)
		assert.Contains(t, v.RepoLinks[0].Label, "default")
		assert.Contains(t, v.RepoLinks[1].Label, "default")
	})
}

func TestStageClass(t *testing.T) {
	assert.Equal(t, "status-online", stageClass("SUCCESS"))
	assert.Equal(t, "status-offline", stageClass("FAILURE"))
	assert.Equal(t, "status-warning", stageClass("STARTED"))
	assert.Equal(t, "status-warning", stageClass("RECEIVED"))
	assert.Equal(t, "", stageClass("PENDING"))
	assert.Equal(t, "", stageClass(""))
}

func TestFormatDuration(t *testing.T) {
	assert.Equal(t, "30s", formatDuration(30*time.Second))
	assert.Equal(t, "5m 30s", formatDuration(5*time.Minute+30*time.Second))
	assert.Equal(t, "2h 30m", formatDuration(2*time.Hour+30*time.Minute))
	assert.Equal(t, "3d 2h", formatDuration(74*time.Hour))
}

func TestFormatRelativeTime(t *testing.T) {
	now := time.Now()

	assert.Equal(t, "just now", formatRelativeTime(now.Add(1*time.Second)))
	assert.Equal(t, "1 second ago", formatRelativeTime(now.Add(-1*time.Second)))
	assert.Equal(t, "30 seconds ago", formatRelativeTime(now.Add(-30*time.Second)))
	assert.Equal(t, "1 minute ago", formatRelativeTime(now.Add(-1*time.Minute)))
	assert.Equal(t, "5 minutes ago", formatRelativeTime(now.Add(-5*time.Minute)))
	assert.Equal(t, "1 hour ago", formatRelativeTime(now.Add(-1*time.Hour)))
	assert.Equal(t, "3 hours ago", formatRelativeTime(now.Add(-3*time.Hour)))
	assert.Equal(t, "1 day ago", formatRelativeTime(now.Add(-24*time.Hour)))
	assert.Equal(t, "7 days ago", formatRelativeTime(now.Add(-7*24*time.Hour)))
}

func TestResolveJobStates(t *testing.T) {
	tq := &mockTaskQueue{
		getTaskStateFn: func(taskName, taskUUID string) string {
			states := map[string]map[string]string{
				"active-job": {"build": "SUCCESS", "repo": "SUCCESS"},
				"stale-job":  {"build": "", "repo": ""},
			}
			if m, ok := states[taskUUID]; ok {
				return m[taskName]
			}
			return ""
		},
	}

	var updatedStages []string
	var updatedStates []string
	js := &mockJobStore{
		updateJobStagesFn: func(taskUUID, buildState, repoState, currentStage string) error {
			updatedStages = append(updatedStages, taskUUID)
			return nil
		},
		updateJobStateFn: func(taskUUID, state string) error {
			updatedStates = append(updatedStates, taskUUID+":"+state)
			return nil
		},
	}

	ds := &DashboardService{
		taskQueue: tq,
		jobStore:  js,
	}

	jobs := []*storage.JobInfo{
		{TaskUUID: "done-job", State: "DONE"},         // terminal, skip
		{TaskUUID: "unknown-job", State: "UNKNOWN"},    // UNKNOWN, skip
		{TaskUUID: "active-job", State: "PENDING"},     // should resolve to DONE
		{TaskUUID: "stale-job", State: "PENDING"},      // both empty, skip
	}

	ds.resolveJobStates(jobs)

	// done-job and unknown-job should be unchanged
	assert.Equal(t, "DONE", jobs[0].State)
	assert.Equal(t, "UNKNOWN", jobs[1].State)

	// active-job should be resolved to DONE
	assert.Equal(t, "DONE", jobs[2].State)
	assert.Equal(t, "SUCCESS", jobs[2].BuildState)
	assert.Equal(t, "SUCCESS", jobs[2].RepoState)
	assert.Equal(t, "completed", jobs[2].CurrentStage)

	// stale-job stays PENDING (both empty)
	assert.Equal(t, "PENDING", jobs[3].State)

	// Only active-job should have been persisted
	assert.Equal(t, []string{"active-job"}, updatedStages)
	assert.Equal(t, []string{"active-job:DONE"}, updatedStates)
}

func TestDashboardService_RenderIndexHTML(t *testing.T) {
	gpg := &mockGPGVerifier{
		listKeysWithColonsFn: func() (string, error) {
			return "", nil
		},
	}
	maintainerSvc := NewMaintainerService(gpg)

	ds, err := NewDashboardService("1.0.0", &mockTaskQueue{}, maintainerSvc, nil, nil, nil)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = ds.RenderIndexHTML(&buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "1.0.0")
}

func TestDashboardService_BuildJobViews_NilJobStore(t *testing.T) {
	ds := &DashboardService{jobStore: nil}
	views := ds.buildJobViews()
	assert.Nil(t, views)
}

func TestDashboardService_BuildISOJobViews_NilISOStore(t *testing.T) {
	ds := &DashboardService{isoStore: nil}
	views := ds.buildISOJobViews()
	assert.Nil(t, views)
}

func TestDashboardService_BuildISOJobViews(t *testing.T) {
	now := time.Now()
	isoStore := &mockISOJobStore{
		getRecentISOJobsFn: func(limit int) ([]*monitoring.ISOJobInfo, error) {
			return []*monitoring.ISOJobInfo{
				{TaskUUID: "iso-1", RepoURL: "https://repo.example.com", Branch: "main", State: "SUCCESS", SubmittedAt: now},
				{TaskUUID: "iso-2", RepoURL: "https://repo.example.com", Branch: "dev", State: "FAILURE", SubmittedAt: now},
				{TaskUUID: "iso-3", RepoURL: "https://repo.example.com", Branch: "test", State: "STARTED", SubmittedAt: now},
				{TaskUUID: "iso-4", RepoURL: "https://repo.example.com", Branch: "test", State: "PENDING", SubmittedAt: now},
			}, nil
		},
	}

	ds := &DashboardService{isoStore: isoStore}
	views := ds.buildISOJobViews()
	require.Len(t, views, 4)
	assert.Equal(t, "status-online", views[0].StatusClass)
	assert.Equal(t, "status-offline", views[1].StatusClass)
	assert.Equal(t, "status-warning", views[2].StatusClass)
	assert.Equal(t, "", views[3].StatusClass)
}
