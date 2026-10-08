package usecase

import (
	"log"
	"net/http"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/blankon/irgsh-go/pkg/httputil"
)

// CancelService stops a job, whether it is still queued or already running.
//
// Machinery cannot withdraw a task once it has been sent, so cancelling is a
// mark in Redis rather than a queue operation: a running worker hears it and
// stops, and a worker that only picks the job up later reads the mark and
// refuses to start it.
type CancelService struct {
	taskQueue   TaskQueue
	signal      CancelSignal
	jobStore    JobStore
	isoStore    ISOJobStore
	importStore ImportJobStore
}

func NewCancelService(
	taskQueue TaskQueue,
	signal CancelSignal,
	jobStore JobStore,
	isoStore ISOJobStore,
	importStore ImportJobStore,
) *CancelService {
	return &CancelService{
		taskQueue:   taskQueue,
		signal:      signal,
		jobStore:    jobStore,
		isoStore:    isoStore,
		importStore: importStore,
	}
}

// IsCanceled reports whether a job was cancelled, so status queries show that
// rather than the FAILURE machinery records when the worker gives up.
func (cs *CancelService) IsCanceled(taskUUID string) bool {
	if cs == nil || cs.signal == nil {
		return false
	}
	return cs.signal.IsRequested(taskUUID)
}

func (cs *CancelService) CancelJob(taskUUID string) (domain.CancelResponse, error) {
	if !domain.SafeIDPattern.MatchString(taskUUID) {
		return domain.CancelResponse{}, httputil.NewHTTPError(http.StatusBadRequest, "invalid pipeline identifier")
	}
	if cs.signal == nil {
		return domain.CancelResponse{}, httputil.NewHTTPError(http.StatusServiceUnavailable,
			"cancellation is unavailable: chief could not reach Redis")
	}

	switch domain.JobKindOf(taskUUID) {
	case domain.KindISO:
		return cs.cancelSingleTask(taskUUID, "iso", cs.storedISOState, cs.updateISOState)
	case domain.KindImport:
		return cs.cancelSingleTask(taskUUID, "import", cs.storedImportState, cs.updateImportState)
	default:
		return cs.cancelPackage(taskUUID)
	}
}

func (cs *CancelService) cancelSingleTask(taskUUID, taskName string, stored func(string) string, persist func(string)) (domain.CancelResponse, error) {
	state := cs.taskQueue.GetTaskState(taskName, taskUUID)
	if state == "" {
		return cs.cancelUntracked(taskUUID, stored, persist)
	}
	if domain.IsFinishedState(state) {
		return domain.CancelResponse{}, httputil.NewHTTPError(http.StatusConflict,
			"job "+taskUUID+" has already finished ("+state+")")
	}
	if cs.IsCanceled(taskUUID) {
		return cs.alreadyCanceled(taskUUID, persist), nil
	}

	return cs.request(taskUUID, state, persist)
}

func (cs *CancelService) cancelPackage(taskUUID string) (domain.CancelResponse, error) {
	buildState := cs.taskQueue.GetTaskState("build", taskUUID)
	repoState := cs.taskQueue.GetTaskState("repo", taskUUID)
	if buildState == "" && repoState == "" {
		return cs.cancelUntracked(taskUUID, cs.storedPackageState, cs.updatePackageState)
	}

	pipelineState := domain.DeriveBuildPipelineState(buildState, repoState)
	if domain.IsFinishedState(pipelineState) {
		return domain.CancelResponse{}, httputil.NewHTTPError(http.StatusConflict,
			"job "+taskUUID+" has already finished ("+pipelineState+")")
	}
	// The repo stage is the one job that cannot be interrupted once it is
	// under way: it drives reprepro, and killing reprepro mid run - during an
	// export above all - can leave the repository database corrupted. Only the
	// build stage of this pipeline is still stoppable, and by then it is done.
	if repoState == "RECEIVED" || repoState == "STARTED" {
		return domain.CancelResponse{}, httputil.NewHTTPError(http.StatusConflict,
			"job "+taskUUID+" is already in its repo stage and cannot be cancelled: "+
				"interrupting reprepro can corrupt the repository database")
	}
	if cs.IsCanceled(taskUUID) {
		return cs.alreadyCanceled(taskUUID, cs.updatePackageState), nil
	}

	return cs.request(taskUUID, pipelineState, cs.updatePackageState)
}

// cancelUntracked handles a job the task queue has no state for. Machinery
// expires task states, so that is not the same as the job never having
// existed: a job whose worker died, or that sat in the queue longer than the
// expiry, is still recorded as running in the job store and the dashboard
// shows it that way forever. The store decides - a row that is still in
// flight is cancelled like any other job, mark included, in case the task is
// in fact still queued.
func (cs *CancelService) cancelUntracked(taskUUID string, stored func(string) string, persist func(string)) (domain.CancelResponse, error) {
	state := stored(taskUUID)
	if state == "" {
		return domain.CancelResponse{}, httputil.NewHTTPError(http.StatusNotFound, "no such job: "+taskUUID)
	}
	if domain.IsFinishedState(state) {
		return domain.CancelResponse{}, httputil.NewHTTPError(http.StatusConflict,
			"job "+taskUUID+" has already finished ("+state+")")
	}

	resp, err := cs.request(taskUUID, state, persist)
	if err != nil {
		return resp, err
	}
	resp.Message = "the task queue no longer tracks this job, it has been marked cancelled"
	return resp, nil
}

// alreadyCanceled answers a repeated cancellation. The mark is there, but the
// stored state may not have followed - the mark can be set without going
// through chief - so it is written again rather than refusing the request.
// A job that has reached a terminal state in the store keeps it.
func (cs *CancelService) alreadyCanceled(taskUUID string, persist func(string)) domain.CancelResponse {
	persist(taskUUID)
	return domain.CancelResponse{
		PipelineID: taskUUID,
		State:      domain.StateCanceled,
		Message:    "job was already cancelled",
	}
}

// request marks the job cancelled and records the new state, so the dashboard
// stops showing it as running without waiting for a worker to report back.
func (cs *CancelService) request(taskUUID, previousState string, persist func(string)) (domain.CancelResponse, error) {
	if err := cs.signal.Request(taskUUID); err != nil {
		log.Printf("Failed to request cancellation of %s: %v\n", taskUUID, err)
		return domain.CancelResponse{}, httputil.NewHTTPError(http.StatusInternalServerError,
			"failed to request cancellation")
	}
	persist(taskUUID)

	message := "job has been cancelled before it started"
	if previousState == "STARTED" || previousState == domain.StateBuilding || previousState == domain.StateRepo {
		message = "cancellation requested, the worker is stopping the job"
	}
	return domain.CancelResponse{
		PipelineID: taskUUID,
		State:      domain.StateCanceled,
		Message:    message,
	}, nil
}

func (cs *CancelService) updatePackageState(taskUUID string) {
	if cs.jobStore == nil {
		return
	}
	if err := cs.jobStore.UpdateJobState(taskUUID, domain.StateCanceled); err != nil {
		log.Printf("Failed to mark job %s cancelled: %v\n", taskUUID, err)
	}
}

func (cs *CancelService) updateISOState(taskUUID string) {
	if cs.isoStore == nil {
		return
	}
	if err := cs.isoStore.UpdateISOJobState(taskUUID, domain.StateCanceled); err != nil {
		log.Printf("Failed to mark ISO job %s cancelled: %v\n", taskUUID, err)
	}
}

func (cs *CancelService) updateImportState(taskUUID string) {
	if cs.importStore == nil {
		return
	}
	if err := cs.importStore.UpdateImportJobState(taskUUID, domain.StateCanceled); err != nil {
		log.Printf("Failed to mark import job %s cancelled: %v\n", taskUUID, err)
	}
}

func (cs *CancelService) storedPackageState(taskUUID string) string {
	if cs.jobStore == nil {
		return ""
	}
	job, err := cs.jobStore.GetJob(taskUUID)
	if err != nil || job == nil {
		return ""
	}
	return job.State
}

func (cs *CancelService) storedISOState(taskUUID string) string {
	if cs.isoStore == nil {
		return ""
	}
	job, err := cs.isoStore.GetISOJob(taskUUID)
	if err != nil || job == nil {
		return ""
	}
	return job.State
}

func (cs *CancelService) storedImportState(taskUUID string) string {
	if cs.importStore == nil {
		return ""
	}
	job, err := cs.importStore.GetImportJob(taskUUID)
	if err != nil || job == nil {
		return ""
	}
	return job.State
}
