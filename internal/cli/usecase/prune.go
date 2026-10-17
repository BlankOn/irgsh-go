package usecase

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/blankon/irgsh-go/internal/cli/domain"
)

func tmpBaseDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".irgsh", "tmp"), nil
}

// PruneTmp removes everything under ~/.irgsh/tmp, where an interrupted or
// failed submission leaves its working tree, tarball and local build scratch.
// An entry that cannot be removed does not stop the others; the errors are
// returned together after the rest has been cleaned.
func (u *CLIUsecase) PruneTmp() (domain.PruneResult, error) {
	base, err := tmpBaseDir()
	if err != nil {
		return domain.PruneResult{}, err
	}

	entries, err := os.ReadDir(base)
	if errors.Is(err, fs.ErrNotExist) {
		return domain.PruneResult{}, nil
	}
	if err != nil {
		return domain.PruneResult{}, err
	}

	var result domain.PruneResult
	var errs []error
	for _, entry := range entries {
		path := filepath.Join(base, entry.Name())
		size := pathSize(path)
		if err := os.RemoveAll(path); err != nil {
			errs = append(errs, err)
			continue
		}
		result.Removed++
		result.Bytes += size
	}
	return result, errors.Join(errs...)
}

func pathSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}
