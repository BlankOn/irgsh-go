package repository

import (
	"path/filepath"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/blankon/irgsh-go/pkg/systemutil"
)

// FileConfigStore persists CLI configuration as individual files under basePath.
type FileConfigStore struct {
	basePath string
}

func NewFileConfigStore(basePath string) *FileConfigStore {
	return &FileConfigStore{basePath: basePath}
}

func (s *FileConfigStore) Load() (domain.Config, error) {
	chief, err := systemutil.ReadFileTrimmed(filepath.Join(s.basePath, "IRGSH_CHIEF_ADDRESS"))
	if err != nil {
		return domain.Config{}, err
	}
	key, err := systemutil.ReadFileTrimmed(filepath.Join(s.basePath, "IRGSH_MAINTAINER_SIGNING_KEY"))
	if err != nil {
		return domain.Config{}, err
	}
	return domain.Config{
		ChiefAddress:         chief,
		MaintainerSigningKey: key,
	}, nil
}

func (s *FileConfigStore) Save(cfg domain.Config) error {
	if err := systemutil.WriteFile(filepath.Join(s.basePath, "IRGSH_CHIEF_ADDRESS"), cfg.ChiefAddress); err != nil {
		return err
	}
	return systemutil.WriteFile(filepath.Join(s.basePath, "IRGSH_MAINTAINER_SIGNING_KEY"), cfg.MaintainerSigningKey)
}
