package usecase_test

import (
	"testing"

	"github.com/blankon/irgsh-go/internal/cli/domain"
	"github.com/blankon/irgsh-go/internal/cli/usecase"
	"github.com/stretchr/testify/assert"
)

func TestSaveConfig_Success(t *testing.T) {
	store := &mockConfigStore{}
	svc := usecase.NewCLIUsecase(store, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	err := svc.SaveConfig(domain.Config{
		ChiefAddress:         "http://example.com",
		MaintainerSigningKey: "ABCD1234",
	})
	assert.NoError(t, err)
	assert.Equal(t, "http://example.com", store.saved.ChiefAddress)
	assert.Equal(t, "ABCD1234", store.saved.MaintainerSigningKey)
}

func TestSaveConfig_EmptyChief(t *testing.T) {
	store := &mockConfigStore{}
	svc := usecase.NewCLIUsecase(store, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	err := svc.SaveConfig(domain.Config{
		MaintainerSigningKey: "ABCD1234",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chief address")
}

func TestSaveConfig_EmptyKey(t *testing.T) {
	store := &mockConfigStore{}
	svc := usecase.NewCLIUsecase(store, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	err := svc.SaveConfig(domain.Config{
		ChiefAddress: "http://example.com",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signing key")
}

func TestSaveConfig_InvalidURL(t *testing.T) {
	store := &mockConfigStore{}
	svc := usecase.NewCLIUsecase(store, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	err := svc.SaveConfig(domain.Config{
		ChiefAddress:         "not-a-url",
		MaintainerSigningKey: "ABCD1234",
	})
	assert.Error(t, err)
}
