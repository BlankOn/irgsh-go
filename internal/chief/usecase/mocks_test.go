package usecase

import (
	"errors"

	"github.com/blankon/irgsh-go/internal/monitoring"
)

// mockTaskQueue implements TaskQueue for testing.
type mockTaskQueue struct {
	sendBuildChainFn func(taskUUID string, payload []byte) error
	sendISOTaskFn    func(taskUUID string, payload []byte) error
	getTaskStateFn   func(taskName, taskUUID string) string
}

func (m *mockTaskQueue) SendBuildChain(taskUUID string, payload []byte) error {
	if m.sendBuildChainFn != nil {
		return m.sendBuildChainFn(taskUUID, payload)
	}
	return nil
}

func (m *mockTaskQueue) SendISOTask(taskUUID string, payload []byte) error {
	if m.sendISOTaskFn != nil {
		return m.sendISOTaskFn(taskUUID, payload)
	}
	return nil
}

func (m *mockTaskQueue) GetTaskState(taskName, taskUUID string) string {
	if m.getTaskStateFn != nil {
		return m.getTaskStateFn(taskName, taskUUID)
	}
	return ""
}

// mockGPGVerifier implements GPGVerifier for testing.
type mockGPGVerifier struct {
	listKeysWithColonsFn      func() (string, error)
	listKeysFn                func() (string, error)
	verifySignedSubmissionFn  func(submissionPath string) error
	verifyFileFn              func(filePath string) error
}

func (m *mockGPGVerifier) ListKeysWithColons() (string, error) {
	if m.listKeysWithColonsFn != nil {
		return m.listKeysWithColonsFn()
	}
	return "", nil
}

func (m *mockGPGVerifier) ListKeys() (string, error) {
	if m.listKeysFn != nil {
		return m.listKeysFn()
	}
	return "", nil
}

func (m *mockGPGVerifier) VerifySignedSubmission(submissionPath string) error {
	if m.verifySignedSubmissionFn != nil {
		return m.verifySignedSubmissionFn(submissionPath)
	}
	return nil
}

func (m *mockGPGVerifier) VerifyFile(filePath string) error {
	if m.verifyFileFn != nil {
		return m.verifyFileFn(filePath)
	}
	return nil
}

// mockFileStorage implements FileStorage for testing.
type mockFileStorage struct {
	artifactsDir   string
	logsDir        string
	submissionsDir string

	ensureDirFn              func(path string) error
	submissionTarballPathFn  func(taskUUID string) string
	submissionDirPathFn      func(taskUUID string) string
	submissionSignaturePathFn func(taskUUID string) string
	extractSubmissionFn      func(taskUUID string) error
	copyFileWithSudoFn       func(src, dst string) error
	copyDirWithSudoFn        func(src, dst string) error
	chownWithSudoFn          func(path string) error
	chownRecursiveWithSudoFn func(path string) error
}

func (m *mockFileStorage) ArtifactsDir() string   { return m.artifactsDir }
func (m *mockFileStorage) LogsDir() string         { return m.logsDir }
func (m *mockFileStorage) SubmissionsDir() string  { return m.submissionsDir }

func (m *mockFileStorage) EnsureDir(path string) error {
	if m.ensureDirFn != nil {
		return m.ensureDirFn(path)
	}
	return nil
}

func (m *mockFileStorage) SubmissionTarballPath(taskUUID string) string {
	if m.submissionTarballPathFn != nil {
		return m.submissionTarballPathFn(taskUUID)
	}
	return m.submissionsDir + "/" + taskUUID + ".tar.gz"
}

func (m *mockFileStorage) SubmissionDirPath(taskUUID string) string {
	if m.submissionDirPathFn != nil {
		return m.submissionDirPathFn(taskUUID)
	}
	return m.submissionsDir + "/" + taskUUID
}

func (m *mockFileStorage) SubmissionSignaturePath(taskUUID string) string {
	if m.submissionSignaturePathFn != nil {
		return m.submissionSignaturePathFn(taskUUID)
	}
	return m.submissionsDir + "/" + taskUUID + ".sig"
}

func (m *mockFileStorage) ExtractSubmission(taskUUID string) error {
	if m.extractSubmissionFn != nil {
		return m.extractSubmissionFn(taskUUID)
	}
	return nil
}

func (m *mockFileStorage) CopyFileWithSudo(src, dst string) error {
	if m.copyFileWithSudoFn != nil {
		return m.copyFileWithSudoFn(src, dst)
	}
	return nil
}

func (m *mockFileStorage) CopyDirWithSudo(src, dst string) error {
	if m.copyDirWithSudoFn != nil {
		return m.copyDirWithSudoFn(src, dst)
	}
	return nil
}

func (m *mockFileStorage) ChownWithSudo(path string) error {
	if m.chownWithSudoFn != nil {
		return m.chownWithSudoFn(path)
	}
	return nil
}

func (m *mockFileStorage) ChownRecursiveWithSudo(path string) error {
	if m.chownRecursiveWithSudoFn != nil {
		return m.chownRecursiveWithSudoFn(path)
	}
	return nil
}

// mockJobStore implements JobStore for testing.
type mockJobStore struct {
	recordJobFn       func(job monitoring.JobInfo) error
	getRecentJobsFn   func(limit int) ([]*monitoring.JobInfo, error)
	getJobFn          func(taskUUID string) (*monitoring.JobInfo, error)
	updateJobStateFn  func(taskUUID string, state string) error
	updateJobStagesFn func(taskUUID, buildState, repoState, currentStage string) error
}

func (m *mockJobStore) RecordJob(job monitoring.JobInfo) error {
	if m.recordJobFn != nil {
		return m.recordJobFn(job)
	}
	return nil
}

func (m *mockJobStore) GetRecentJobs(limit int) ([]*monitoring.JobInfo, error) {
	if m.getRecentJobsFn != nil {
		return m.getRecentJobsFn(limit)
	}
	return nil, nil
}

func (m *mockJobStore) GetJob(taskUUID string) (*monitoring.JobInfo, error) {
	if m.getJobFn != nil {
		return m.getJobFn(taskUUID)
	}
	return nil, errors.New("not found")
}

func (m *mockJobStore) UpdateJobState(taskUUID string, state string) error {
	if m.updateJobStateFn != nil {
		return m.updateJobStateFn(taskUUID, state)
	}
	return nil
}

func (m *mockJobStore) UpdateJobStages(taskUUID, buildState, repoState, currentStage string) error {
	if m.updateJobStagesFn != nil {
		return m.updateJobStagesFn(taskUUID, buildState, repoState, currentStage)
	}
	return nil
}

// mockISOJobStore implements ISOJobStore for testing.
type mockISOJobStore struct {
	recordISOJobFn     func(job monitoring.ISOJobInfo) error
	getRecentISOJobsFn func(limit int) ([]*monitoring.ISOJobInfo, error)
}

func (m *mockISOJobStore) RecordISOJob(job monitoring.ISOJobInfo) error {
	if m.recordISOJobFn != nil {
		return m.recordISOJobFn(job)
	}
	return nil
}

func (m *mockISOJobStore) GetRecentISOJobs(limit int) ([]*monitoring.ISOJobInfo, error) {
	if m.getRecentISOJobsFn != nil {
		return m.getRecentISOJobsFn(limit)
	}
	return nil, nil
}

// mockInstanceRegistry implements InstanceRegistry for testing.
type mockInstanceRegistry struct {
	listInstancesFn func(instanceType monitoring.InstanceType, status monitoring.InstanceStatus) ([]*monitoring.InstanceInfo, error)
	getSummaryFn    func() (monitoring.InstanceSummary, error)
}

func (m *mockInstanceRegistry) ListInstances(instanceType monitoring.InstanceType, status monitoring.InstanceStatus) ([]*monitoring.InstanceInfo, error) {
	if m.listInstancesFn != nil {
		return m.listInstancesFn(instanceType, status)
	}
	return nil, nil
}

func (m *mockInstanceRegistry) GetSummary() (monitoring.InstanceSummary, error) {
	if m.getSummaryFn != nil {
		return m.getSummaryFn()
	}
	return monitoring.InstanceSummary{}, nil
}
