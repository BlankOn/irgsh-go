package repository

import (
	"path/filepath"

	"github.com/blankon/irgsh-go/pkg/systemutil"
)

// FilePipelineStore persists last-used pipeline IDs as files under basePath.
type FilePipelineStore struct {
	basePath string
}

func NewFilePipelineStore(basePath string) *FilePipelineStore {
	return &FilePipelineStore{basePath: basePath}
}

func (s *FilePipelineStore) SavePackageID(id string) error {
	return systemutil.WriteFile(filepath.Join(s.basePath, "LAST_PACKAGE_PIPELINE_ID"), id)
}

func (s *FilePipelineStore) LoadPackageID() (string, error) {
	return systemutil.ReadFileTrimmed(filepath.Join(s.basePath, "LAST_PACKAGE_PIPELINE_ID"))
}

func (s *FilePipelineStore) SaveISOID(id string) error {
	return systemutil.WriteFile(filepath.Join(s.basePath, "LAST_ISO_PIPELINE_ID"), id)
}

func (s *FilePipelineStore) LoadISOID() (string, error) {
	return systemutil.ReadFileTrimmed(filepath.Join(s.basePath, "LAST_ISO_PIPELINE_ID"))
}

func (s *FilePipelineStore) SaveRetryID(id string) error {
	return systemutil.WriteFile(filepath.Join(s.basePath, "LAST_PIPELINE_ID"), id)
}

func (s *FilePipelineStore) LoadRetryID() (string, error) {
	return systemutil.ReadFileTrimmed(filepath.Join(s.basePath, "LAST_PIPELINE_ID"))
}
