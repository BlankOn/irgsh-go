package usecase

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/blankon/irgsh-go/internal/monitoring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockImportJobStore records what the service persists.
type mockImportJobStore struct {
	recorded      []monitoring.ImportJobInfo
	recent        []*monitoring.ImportJobInfo
	updatedStates map[string]string
	err           error
}

func (m *mockImportJobStore) RecordImportJob(job monitoring.ImportJobInfo) error {
	m.recorded = append(m.recorded, job)
	return m.err
}

func (m *mockImportJobStore) GetRecentImportJobs(int) ([]*monitoring.ImportJobInfo, error) {
	return m.recent, m.err
}

func (m *mockImportJobStore) UpdateImportJobState(taskUUID, state string) error {
	if m.updatedStates == nil {
		m.updatedStates = map[string]string{}
	}
	m.updatedStates[taskUUID] = state
	return nil
}

func validImportSubmission() domain.ImportSubmission {
	return domain.ImportSubmission{
		SourceURL:    "https://kartolo.sby.datautama.net.id/debian/",
		Dist:         "sid",
		TargetDist:   "verbeek",
		PackageNames: []string{"grub-efi-amd64-bin", "calamares"},
		Maintainer:   "Herpiko Dwi Aguno <herpiko@gmail.com>",
	}
}

func TestImportPackages_ValidationErrors(t *testing.T) {
	cases := map[string]struct {
		mutate func(*domain.ImportSubmission)
		want   string
	}{
		"missing source":     {func(s *domain.ImportSubmission) { s.SourceURL = "" }, "sourceUrl is required"},
		"malformed source":   {func(s *domain.ImportSubmission) { s.SourceURL = "not a url" }, "not a valid URL"},
		"missing dist":       {func(s *domain.ImportSubmission) { s.Dist = "" }, "dist is required"},
		"unsafe dist":        {func(s *domain.ImportSubmission) { s.Dist = "sid; rm -rf /" }, "unsupported characters"},
		"missing targetDist": {func(s *domain.ImportSubmission) { s.TargetDist = "" }, "targetDist is required"},
		"unsafe targetDist":  {func(s *domain.ImportSubmission) { s.TargetDist = "verbeek; rm -rf /" }, "unsupported characters"},
		"no packages":        {func(s *domain.ImportSubmission) { s.PackageNames = nil }, "packageNames is required"},
		"unsafe package":     {func(s *domain.ImportSubmission) { s.PackageNames = []string{"grub$(id)"} }, "invalid package name"},
		"unsafe component":   {func(s *domain.ImportSubmission) { s.Component = "main;evil" }, "invalid component"},
		"unsafe src compnt":  {func(s *domain.ImportSubmission) { s.SourceComponent = "../../etc" }, "invalid component"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := newTestImportService(&mockTaskQueue{}, nil)
			submission := validImportSubmission()
			tc.mutate(&submission)

			_, err := svc.ImportPackages(submission)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestImportPackages_QueuesTaskAndRecordsJob(t *testing.T) {
	var queuedUUID string
	var queuedPayload []byte
	tq := &mockTaskQueue{
		sendImportTaskFn: func(taskUUID, dist string, payload []byte) error {
			queuedUUID = taskUUID
			queuedPayload = payload
			return nil
		},
	}
	store := &mockImportJobStore{}
	svc := newTestImportService(tq, store)

	resp, err := svc.ImportPackages(validImportSubmission())
	require.NoError(t, err)

	assert.Equal(t, queuedUUID, resp.PipelineID)
	assert.True(t, strings.HasSuffix(resp.PipelineID, "_import"),
		"the task UUID must be recognisable as an import job: %s", resp.PipelineID)

	// The worker receives the submission with the defaults applied.
	var queued domain.ImportSubmission
	require.NoError(t, json.Unmarshal(queuedPayload, &queued))
	assert.Equal(t, "main", queued.Component)
	assert.Equal(t, "main", queued.SourceComponent)
	assert.Equal(t, []string{"grub-efi-amd64-bin", "calamares"}, queued.PackageNames)
	assert.Equal(t, queuedUUID, queued.TaskUUID)
	assert.False(t, queued.Timestamp.IsZero())

	require.Len(t, store.recorded, 1)
	recorded := store.recorded[0]
	assert.Equal(t, queuedUUID, recorded.TaskUUID)
	assert.Equal(t, "grub-efi-amd64-bin, calamares", recorded.Packages)
	assert.Equal(t, "sid", recorded.Dist)
	assert.Equal(t, "PENDING", recorded.State)
	assert.Equal(t, "Herpiko Dwi Aguno <herpiko@gmail.com>", recorded.Maintainer,
		"the dashboard needs to show who triggered the import")
}

func TestImportPackages_QueueFailureIsReported(t *testing.T) {
	tq := &mockTaskQueue{
		sendImportTaskFn: func(string, string, []byte) error { return assert.AnError },
	}
	store := &mockImportJobStore{}

	_, err := newTestImportService(tq, store).ImportPackages(validImportSubmission())
	require.Error(t, err)
	assert.Empty(t, store.recorded, "a job that was never queued must not be recorded")
}

func TestImportPackages_WithoutJobStore(t *testing.T) {
	// Monitoring may be disabled; the import must still be queued.
	resp, err := newTestImportService(&mockTaskQueue{}, nil).ImportPackages(validImportSubmission())
	require.NoError(t, err)
	assert.NotEmpty(t, resp.PipelineID)
}

func TestDashboardImportJobState_ResolvedFromTaskQueue(t *testing.T) {
	now := time.Now()
	store := &mockImportJobStore{
		recent: []*monitoring.ImportJobInfo{
			{TaskUUID: "running", Packages: "firefox", State: "PENDING", SubmittedAt: now},
			{TaskUUID: "failed", Packages: "grub-pc", State: "PENDING", SubmittedAt: now},
			{TaskUUID: "finished", Packages: "calamares", State: "SUCCESS", SubmittedAt: now},
			{TaskUUID: "expired", Packages: "hello", State: "PENDING", SubmittedAt: now},
		},
	}
	tq := &mockTaskQueue{
		getTaskStateFn: func(taskName, taskUUID string) string {
			if taskName != "import" {
				t.Fatalf("expected the import task to be queried, got %q", taskName)
			}
			switch taskUUID {
			case "running":
				return "STARTED"
			case "failed":
				return "FAILURE"
			case "expired":
				return "" // machinery has expired the result
			}
			return ""
		},
	}

	ds := &DashboardService{importStore: store, taskQueue: tq}
	views := ds.buildImportJobViews()
	require.Len(t, views, 4)

	// A job the worker has already failed must not stay PENDING on the board.
	assert.Equal(t, "STARTED", views[0].State)
	assert.Equal(t, "FAILURE", views[1].State)
	assert.Equal(t, "status-offline", views[1].StatusClass)
	// A terminal state is never re-queried, and an expired result is kept.
	assert.Equal(t, "SUCCESS", views[2].State)
	assert.Equal(t, "PENDING", views[3].State)

	// The resolved states are written back so they survive result expiry.
	assert.Equal(t, "STARTED", store.updatedStates["running"])
	assert.Equal(t, "FAILURE", store.updatedStates["failed"])
	assert.NotContains(t, store.updatedStates, "finished")
	assert.NotContains(t, store.updatedStates, "expired")
}

func TestImportPackages_KeyringPathValidation(t *testing.T) {
	for _, bad := range []string{
		"relative/keyring.gpg",
		"/usr/share/keyrings/archive.txt",
		"/usr/share/keyrings/$(id).gpg",
		"/usr/share/keyrings/a;rm -rf /.gpg",
	} {
		submission := validImportSubmission()
		submission.KeyringPath = bad
		_, err := newTestImportService(&mockTaskQueue{}, nil).ImportPackages(submission)
		require.Error(t, err, "keyringPath %q must be rejected", bad)
		assert.Contains(t, err.Error(), "keyringPath")
	}

	submission := validImportSubmission()
	submission.KeyringPath = "/usr/share/keyrings/debian-archive-keyring.gpg"
	_, err := newTestImportService(&mockTaskQueue{}, nil).ImportPackages(submission)
	require.NoError(t, err)
}

func TestFormatPackageList(t *testing.T) {
	cases := map[string]string{
		// Rows written before the list was stored comma separated.
		"grub-efi-amd64-bin grub-pc":  "grub-efi-amd64-bin, grub-pc",
		"grub-efi-amd64-bin, grub-pc": "grub-efi-amd64-bin, grub-pc",
		"firefox":                     "firefox",
		"":                            "",
	}
	for input, want := range cases {
		assert.Equal(t, want, formatPackageList(input), "input %q", input)
	}
}

func TestImportJobView_SpinnerAndImporter(t *testing.T) {
	now := time.Now()
	store := &mockImportJobStore{
		recent: []*monitoring.ImportJobInfo{
			{TaskUUID: "a", Packages: "grub-efi-amd64-bin grub-pc", Maintainer: "Herpiko Dwi Aguno <herpiko@gmail.com>", State: "STARTED", SubmittedAt: now},
			{TaskUUID: "b", Packages: "firefox", Maintainer: "Herpiko Dwi Aguno <herpiko@gmail.com>", State: "SUCCESS", SubmittedAt: now},
			{TaskUUID: "c", Packages: "firefox", Maintainer: "Herpiko Dwi Aguno <herpiko@gmail.com>", State: "FAILURE", SubmittedAt: now},
			{TaskUUID: "d", Packages: "firefox", Maintainer: "Herpiko Dwi Aguno <herpiko@gmail.com>", State: "PENDING", SubmittedAt: now},
		},
	}
	ds := &DashboardService{importStore: store}
	views := ds.buildImportJobViews()
	require.Len(t, views, 4)

	// A running job shows the spinner, exactly like a packaging job.
	assert.True(t, views[0].ShowSpinner, "STARTED must spin")
	assert.False(t, views[1].ShowSpinner, "SUCCESS must not spin")
	assert.False(t, views[2].ShowSpinner, "FAILURE must not spin")
	assert.True(t, views[3].ShowSpinner, "PENDING must spin")

	assert.Equal(t, "grub-efi-amd64-bin, grub-pc", views[0].Packages)
	assert.Equal(t, "Herpiko Dwi Aguno <herpiko@gmail.com>", views[0].Maintainer)
}
