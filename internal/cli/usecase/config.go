package usecase

import (
	"errors"
	"net/url"

	"github.com/blankon/irgsh-go/internal/cli/entity"
)

func (u *CLIUsecase) SaveConfig(cfg entity.Config) error {
	if cfg.ChiefAddress == "" {
		return errors.New("chief address should not be empty")
	}
	if cfg.MaintainerSigningKey == "" {
		return errors.New("signing key should not be empty")
	}
	if _, err := url.ParseRequestURI(cfg.ChiefAddress); err != nil {
		return err
	}
	return u.Config.Save(cfg)
}
