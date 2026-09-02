package usecase

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/blankon/irgsh-go/internal/monitoring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockImportJobStore records what the service persists.
type mockImportJobStore struct {
	recorded []monitoring.ImportJobInfo
	recent   []*monitoring.ImportJobInfo
	err      error
}

func (m *mockImportJobStore) RecordImportJob(job monitoring.ImportJobInfo) error {
	m.recorded = append(m.recorded, job)
	return m.err
}

func (m *mockImportJobStore) GetRecentImportJobs(int) ([]*monitoring.ImportJobInfo, error) {
	return m.recent, m.err
}

func validImportSubmission() domain.ImportSubmission {
	return domain.ImportSubmission{
		SourceURL:    "https://kartolo.sby.datautama.net.id/debian/",
		Dist:         "sid",
		PackageNames: []string{"grub-efi-amd64-bin", "calamares"},
	}
}

func TestImportPackages_ValidationErrors(t *testing.T) {
	cases := map[string]struct {
		mutate func(*domain.ImportSubmission)
		want   string
	}{
		"missing source":    {func(s *domain.ImportSubmission) { s.SourceURL = "" }, "sourceUrl is required"},
		"malformed source":  {func(s *domain.ImportSubmission) { s.SourceURL = "not a url" }, "not a valid URL"},
		"missing dist":      {func(s *domain.ImportSubmission) { s.Dist = "" }, "dist is required"},
		"unsafe dist":       {func(s *domain.ImportSubmission) { s.Dist = "sid; rm -rf /" }, "unsupported characters"},
		"no packages":       {func(s *domain.ImportSubmission) { s.PackageNames = nil }, "packageNames is required"},
		"unsafe package":    {func(s *domain.ImportSubmission) { s.PackageNames = []string{"grub$(id)"} }, "invalid package name"},
		"unsafe component":  {func(s *domain.ImportSubmission) { s.Component = "main;evil" }, "invalid component"},
		"unsafe src compnt": {func(s *domain.ImportSubmission) { s.SourceComponent = "../../etc" }, "invalid component"},
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
		sendImportTaskFn: func(taskUUID string, payload []byte) error {
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
	assert.Equal(t, "grub-efi-amd64-bin calamares", recorded.Packages)
	assert.Equal(t, "sid", recorded.Dist)
	assert.Equal(t, "PENDING", recorded.State)
}

func TestImportPackages_QueueFailureIsReported(t *testing.T) {
	tq := &mockTaskQueue{
		sendImportTaskFn: func(string, []byte) error { return assert.AnError },
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
