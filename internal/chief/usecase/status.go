package usecase

import (
	"github.com/blankon/irgsh-go/internal/chief/domain"
)

// StatusService handles build and ISO status queries.
type StatusService struct {
	taskQueue TaskQueue
	cancels   *CancelService
}

func NewStatusService(taskQueue TaskQueue, cancels *CancelService) *StatusService {
	return &StatusService{taskQueue: taskQueue, cancels: cancels}
}

// pipelineState reports the state a client should see. A cancelled job is
// reported as such rather than as the FAILURE machinery records once the
// worker abandons it.
func (st *StatusService) pipelineState(UUID, derived string) string {
	if st.cancels.IsCanceled(UUID) {
		return domain.StateCanceled
	}
	return derived
}

func (st *StatusService) BuildStatus(UUID string) (domain.BuildStatusResponse, error) {
	buildState := st.taskQueue.GetTaskState("build", UUID)
	repoState := st.taskQueue.GetTaskState("repo", UUID)
	pipelineState := st.pipelineState(UUID, domain.DeriveBuildPipelineState(buildState, repoState))

	return domain.BuildStatusResponse{
		PipelineID:  UUID,
		JobStatus:   pipelineState,
		BuildStatus: buildState,
		RepoStatus:  repoState,
		State:       pipelineState,
	}, nil
}

func (st *StatusService) ISOStatus(UUID string) (string, string, error) {
	isoStatusStr := st.taskQueue.GetTaskState("iso", UUID)
	jobStatus := st.pipelineState(UUID, domain.DeriveISOPipelineState(isoStatusStr))
	return jobStatus, isoStatusStr, nil
}

func (st *StatusService) ImportStatus(UUID string) (string, string, error) {
	importStatusStr := st.taskQueue.GetTaskState("import", UUID)
	jobStatus := st.pipelineState(UUID, domain.DeriveImportPipelineState(importStatusStr))
	return jobStatus, importStatusStr, nil
}
