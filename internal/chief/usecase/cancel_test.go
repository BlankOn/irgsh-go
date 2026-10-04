package usecase

import (
	"errors"
	"net/http"
	"testing"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/blankon/irgsh-go/pkg/httputil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCancelSignal records what was cancelled.
type mockCancelSignal struct {
	requested map[string]bool
	err       error
}

func (m *mockCancelSignal) Request(taskUUID string) error {
	if m.err != nil {
		return m.err
	}
	if m.requested == nil {
		m.requested = map[string]bool{}
	}
	m.requested[taskUUID] = true
	return nil
}

func (m *mockCancelSignal) IsRequested(taskUUID string) bool { return m.requested[taskUUID] }

func statusCodeOf(t *testing.T, err error) int {
	t.Helper()
	var httpErr httputil.HTTPError
	require.True(t, errors.As(err, &httpErr), "expected an HTTP error, got %v", err)
	return httpErr.Code
}

func TestJobKindOf(t *testing.T) {
	assert.Equal(t, domain.KindISO, domain.JobKindOf("2026-09-10-134726_46ef4607-d39e_iso"))
	assert.Equal(t, domain.KindImport, domain.JobKindOf("2026-09-10-134726_46ef4607-d39e_import"))
	assert.Equal(t, domain.KindPackage, domain.JobKindOf("2026-09-10-134726_46ef4607-d39e_ABCDEF_nano"))
	// A package whose name happens to be "iso" still has four fields.
	assert.Equal(t, domain.KindPackage, domain.JobKindOf("2026-09-10-134726_46ef4607-d39e_ABCDEF_iso"))
}

func TestCancelService_ImportJob(t *testing.T) {
	tq := &mockTaskQueue{getTaskStateFn: func(string, string) string { return "STARTED" }}
	signal := &mockCancelSignal{}
	store := &mockImportJobStore{}
	svc := NewCancelService(tq, signal, nil, nil, store)

	const uuid = "2026-09-10-134726_46ef4607-d39e_import"
	resp, err := svc.CancelJob(uuid)
	require.NoError(t, err)
	assert.Equal(t, domain.StateCanceled, resp.State)
	assert.True(t, signal.requested[uuid])
	// The dashboard reads this, so the job stops showing as running without
	// waiting for the worker to report back.
	assert.Equal(t, domain.StateCanceled, store.updatedStates[uuid])
}

func TestCancelService_ISOJob(t *testing.T) {
	tq := &mockTaskQueue{getTaskStateFn: func(string, string) string { return "PENDING" }}
	signal := &mockCancelSignal{}
	store := &mockISOJobStore{}
	svc := NewCancelService(tq, signal, nil, store, nil)

	const uuid = "2026-09-10-134726_46ef4607-d39e_iso"
	resp, err := svc.CancelJob(uuid)
	require.NoError(t, err)
	assert.Equal(t, domain.StateCanceled, resp.State)
	assert.Equal(t, domain.StateCanceled, store.updatedStates[uuid])
}

func TestCancelService_PackageDuringBuild(t *testing.T) {
	tq := &mockTaskQueue{getTaskStateFn: func(taskName, _ string) string {
		if taskName == "build" {
			return "STARTED"
		}
		return "PENDING"
	}}
	signal := &mockCancelSignal{}
	var persisted string
	store := &mockJobStore{updateJobStateFn: func(_, state string) error {
		persisted = state
		return nil
	}}
	svc := NewCancelService(tq, signal, store, nil, nil)

	const uuid = "2026-09-10-134726_46ef4607-d39e_ABCDEF_nano"
	resp, err := svc.CancelJob(uuid)
	require.NoError(t, err)
	assert.Equal(t, domain.StateCanceled, resp.State)
	assert.Equal(t, domain.StateCanceled, persisted)
}

// A pipeline already in its repo stage is the one job that cannot be stopped:
// interrupting reprepro can corrupt the repository database.
func TestCancelService_PackageInRepoStageRefused(t *testing.T) {
	for _, repoState := range []string{"RECEIVED", "STARTED"} {
		tq := &mockTaskQueue{getTaskStateFn: func(taskName, _ string) string {
			if taskName == "build" {
				return "SUCCESS"
			}
			return repoState
		}}
		signal := &mockCancelSignal{}
		svc := NewCancelService(tq, signal, &mockJobStore{}, nil, nil)

		_, err := svc.CancelJob("2026-09-10-134726_46ef4607-d39e_ABCDEF_nano")
		require.Error(t, err, "repo state %s", repoState)
		assert.Equal(t, http.StatusConflict, statusCodeOf(t, err))
		assert.Contains(t, err.Error(), "reprepro")
		assert.Empty(t, signal.requested)
	}
}

func TestCancelService_FinishedJobRefused(t *testing.T) {
	tq := &mockTaskQueue{getTaskStateFn: func(string, string) string { return "SUCCESS" }}
	signal := &mockCancelSignal{}
	svc := NewCancelService(tq, signal, nil, nil, &mockImportJobStore{})

	_, err := svc.CancelJob("2026-09-10-134726_46ef4607-d39e_import")
	require.Error(t, err)
	assert.Equal(t, http.StatusConflict, statusCodeOf(t, err))
	assert.Empty(t, signal.requested)
}

func TestCancelService_UnknownJob(t *testing.T) {
	tq := &mockTaskQueue{}
	svc := NewCancelService(tq, &mockCancelSignal{}, &mockJobStore{}, nil, nil)

	_, err := svc.CancelJob("2026-09-10-134726_46ef4607-d39e_ABCDEF_nano")
	require.Error(t, err)
	assert.Equal(t, http.StatusNotFound, statusCodeOf(t, err))
}

func TestCancelService_InvalidID(t *testing.T) {
	svc := NewCancelService(&mockTaskQueue{}, &mockCancelSignal{}, nil, nil, nil)

	_, err := svc.CancelJob("../../etc/passwd")
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCodeOf(t, err))
}

// Without Redis there is no way to reach the workers, so the request is
// refused rather than silently doing nothing.
func TestCancelService_WithoutSignal(t *testing.T) {
	svc := NewCancelService(&mockTaskQueue{}, nil, nil, nil, nil)

	_, err := svc.CancelJob("2026-09-10-134726_46ef4607-d39e_iso")
	require.Error(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, statusCodeOf(t, err))
}

// A cancelled job reports CANCELED rather than the FAILURE machinery records
// once the worker gives up on it.
func TestStatusService_ReportsCanceled(t *testing.T) {
	const uuid = "2026-09-10-134726_46ef4607-d39e_ABCDEF_nano"
	tq := &mockTaskQueue{getTaskStateFn: func(string, string) string { return "FAILURE" }}
	signal := &mockCancelSignal{requested: map[string]bool{uuid: true}}
	cancels := NewCancelService(tq, signal, nil, nil, nil)

	resp, err := NewStatusService(tq, cancels).BuildStatus(uuid)
	require.NoError(t, err)
	assert.Equal(t, domain.StateCanceled, resp.State)
	assert.Equal(t, domain.StateCanceled, resp.JobStatus)
	// The stage states are still reported as they are.
	assert.Equal(t, "FAILURE", resp.BuildStatus)
}
