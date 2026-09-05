package usecase

import (
	"fmt"
	"io"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	chiefrepository "github.com/blankon/irgsh-go/internal/chief/repository"
	"github.com/blankon/irgsh-go/internal/config"
	"github.com/blankon/irgsh-go/internal/monitoring"
)

type ChiefUsecase struct {
	config             config.IrgshConfig
	taskQueue          TaskQueue
	monitoringRegistry *monitoring.Registry
	storage            *chiefrepository.Storage
	gpg                *chiefrepository.GPG
	version            string
	maintainerSvc      *MaintainerService
	uploadSvc          *UploadService
	statusSvc          *StatusService
	submissionSvc      *SubmissionService
	dashboardSvc       *DashboardService
}

func NewChiefUsecase(
	cfg config.IrgshConfig,
	taskQueue TaskQueue,
	registry *monitoring.Registry,
	storage *chiefrepository.Storage,
	gpg *chiefrepository.GPG,
	version string,
) (*ChiefUsecase, error) {
	maintainerSvc := NewMaintainerService(gpg)
	dashSvc, err := newDashboardSvc(version, taskQueue, maintainerSvc, registry)
	if err != nil {
		return nil, fmt.Errorf("init dashboard service: %w", err)
	}
	return &ChiefUsecase{
		config:             cfg,
		taskQueue:          taskQueue,
		monitoringRegistry: registry,
		storage:            storage,
		gpg:                gpg,
		version:            version,
		maintainerSvc:      maintainerSvc,
		uploadSvc:          NewUploadService(storage, gpg),
		statusSvc:          NewStatusService(taskQueue),
		submissionSvc:      newSubmissionSvc(taskQueue, storage, gpg, registry),
		dashboardSvc:       dashSvc,
	}, nil
}

// newSubmissionSvc constructs a SubmissionService, avoiding a non-nil
// interface wrapping a nil *Registry pointer.
func newSubmissionSvc(tq TaskQueue, st FileStorage, gpg GPGVerifier, reg *monitoring.Registry) *SubmissionService {
	var js JobStore
	var is ISOJobStore
	var imp ImportJobStore
	if reg != nil {
		js = reg
		is = reg
		imp = reg
	}
	return NewSubmissionService(tq, st, gpg, js, is, imp)
}

func newDashboardSvc(version string, tq TaskQueue, ms *MaintainerService, reg *monitoring.Registry) (*DashboardService, error) {
	var ir InstanceRegistry
	var js JobStore
	var is ISOJobStore
	var imp ImportJobStore
	if reg != nil {
		ir = reg
		js = reg
		is = reg
		imp = reg
	}
	return NewDashboardService(version, tq, ms, ir, js, is, imp)
}

// GetVersion returns the version string for use by handlers.
func (s *ChiefUsecase) GetVersion() string {
	return s.version
}

func (s *ChiefUsecase) GetMaintainers() []domain.Maintainer {
	return s.maintainerSvc.GetMaintainers()
}

func (s *ChiefUsecase) RenderIndexHTML(w io.Writer) error {
	return s.dashboardSvc.RenderIndexHTML(w)
}

func (s *ChiefUsecase) RenderLogViewerHTML(w io.Writer, taskUUID, logType string) error {
	return s.dashboardSvc.RenderLogViewerHTML(w, taskUUID, logType)
}

func (s *ChiefUsecase) LogoPNG() []byte {
	return s.dashboardSvc.LogoPNG()
}

func (s *ChiefUsecase) ImportPackages(submission domain.ImportSubmission) (domain.SubmitPayloadResponse, error) {
	return s.submissionSvc.ImportPackages(submission)
}

// RepoInfo reports where a given distribution's repository is published and
// under which codename. Chief holds no repo config of its own (it is
// distribution-agnostic); it answers from the online repo instance that
// advertised this dist over its heartbeat.
func (s *ChiefUsecase) RepoInfo(dist string) domain.RepoInfo {
	if s.monitoringRegistry == nil || dist == "" {
		return domain.RepoInfo{}
	}
	instances, err := s.monitoringRegistry.ListInstances(monitoring.InstanceTypeRepo, monitoring.StatusOnline)
	if err != nil {
		return domain.RepoInfo{}
	}
	for _, inst := range instances {
		if inst.Dist == dist {
			return domain.RepoInfo{
				PublicURL:      inst.PublicURL,
				DistCodename:   inst.Dist,
				DistComponents: inst.DistComponents,
			}
		}
	}
	return domain.RepoInfo{}
}

func (s *ChiefUsecase) ImportStatus(UUID string) (string, string, error) {
	return s.statusSvc.ImportStatus(UUID)
}

func (s *ChiefUsecase) SubmitPackage(submission domain.Submission) (domain.SubmitPayloadResponse, error) {
	return s.submissionSvc.SubmitPackage(submission)
}

func (s *ChiefUsecase) BuildStatus(UUID string) (domain.BuildStatusResponse, error) {
	return s.statusSvc.BuildStatus(UUID)
}

func (s *ChiefUsecase) ISOStatus(UUID string) (string, string, error) {
	return s.statusSvc.ISOStatus(UUID)
}

func (s *ChiefUsecase) RetryPipeline(oldTaskUUID string) (domain.SubmitPayloadResponse, error) {
	return s.submissionSvc.RetryPipeline(oldTaskUUID)
}

func (s *ChiefUsecase) UploadArtifact(id string, file io.Reader) error {
	return s.uploadSvc.UploadArtifact(id, file)
}

func (s *ChiefUsecase) UploadLog(id string, logType string, file io.Reader) error {
	return s.uploadSvc.UploadLog(id, logType, file)
}

func (s *ChiefUsecase) BuildISO(submission domain.ISOSubmission) (domain.SubmitPayloadResponse, error) {
	return s.submissionSvc.BuildISO(submission)
}

func (s *ChiefUsecase) UploadSubmission(tokenData []byte, blob io.Reader) (string, error) {
	return s.uploadSvc.UploadSubmission(tokenData, blob)
}

func (s *ChiefUsecase) ListMaintainersRaw() (string, error) {
	return s.maintainerSvc.ListMaintainersRaw()
}
