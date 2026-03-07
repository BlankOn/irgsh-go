package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusService_BuildStatus(t *testing.T) {
	tests := []struct {
		name       string
		buildState string
		repoState  string
		wantState  string
	}{
		{"both success", "SUCCESS", "SUCCESS", "DONE"},
		{"build failure", "FAILURE", "", "FAILED"},
		{"build success, repo failure", "SUCCESS", "FAILURE", "FAILED"},
		{"build pending", "PENDING", "", "BUILDING"},
		{"build started", "STARTED", "", "BUILDING"},
		{"both empty", "", "", "UNKNOWN"},
		{"build success, repo pending", "SUCCESS", "PENDING", "BUILDING"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tq := &mockTaskQueue{
				getTaskStateFn: func(taskName, taskUUID string) string {
					switch taskName {
					case "build":
						return tt.buildState
					case "repo":
						return tt.repoState
					}
					return ""
				},
			}

			svc := NewStatusService(tq)
			resp, err := svc.BuildStatus("test-uuid")
			require.NoError(t, err)
			assert.Equal(t, "test-uuid", resp.PipelineID)
			assert.Equal(t, tt.wantState, resp.State)
			assert.Equal(t, tt.wantState, resp.JobStatus)
			assert.Equal(t, tt.buildState, resp.BuildStatus)
			assert.Equal(t, tt.repoState, resp.RepoStatus)
		})
	}
}

func TestStatusService_ISOStatus(t *testing.T) {
	tests := []struct {
		name          string
		isoState      string
		wantJobStatus string
	}{
		{"success", "SUCCESS", "DONE"},
		{"failure", "FAILURE", "FAILED"},
		{"pending", "PENDING", "BUILDING"},
		{"started", "STARTED", "BUILDING"},
		{"empty", "", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tq := &mockTaskQueue{
				getTaskStateFn: func(taskName, taskUUID string) string {
					assert.Equal(t, "iso", taskName)
					return tt.isoState
				},
			}

			svc := NewStatusService(tq)
			jobStatus, rawState, err := svc.ISOStatus("iso-uuid")
			require.NoError(t, err)
			assert.Equal(t, tt.wantJobStatus, jobStatus)
			assert.Equal(t, tt.isoState, rawState)
		})
	}
}
