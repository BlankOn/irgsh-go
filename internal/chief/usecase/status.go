package usecase

import (
	"github.com/blankon/irgsh-go/internal/chief/domain"
)

// StatusService handles build and ISO status queries.
type StatusService struct {
	taskQueue TaskQueue
}

func NewStatusService(taskQueue TaskQueue) *StatusService {
	return &StatusService{taskQueue: taskQueue}
}

func (st *StatusService) BuildStatus(UUID string) (domain.BuildStatusResponse, error) {
	buildState := st.taskQueue.GetTaskState("build", UUID)
	repoState := st.taskQueue.GetTaskState("repo", UUID)
	pipelineState := domain.DeriveBuildPipelineState(buildState, repoState)

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
	jobStatus := domain.DeriveISOPipelineState(isoStatusStr)
	return jobStatus, isoStatusStr, nil
}
