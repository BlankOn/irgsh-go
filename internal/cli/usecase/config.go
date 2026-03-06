package usecase

import (
	"errors"
	"net/url"

	"github.com/blankon/irgsh-go/internal/cli/domain"
)

func (u *CLIUsecase) SaveConfig(cfg domain.Config) error {
	if cfg.ChiefAddress == "" {
		return errors.New("chief address should not be empty")
	}
	if cfg.MaintainerSigningKey == "" {
		return errors.New("signing key should not be empty")
	}
	parsed, err := url.Parse(cfg.ChiefAddress)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("chief address must be a valid URL with scheme and host")
	}
	return u.config.Save(cfg)
}
