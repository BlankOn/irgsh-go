package usecase

import (
	"errors"
	"testing"

	"github.com/blankon/irgsh-go/internal/chief/domain"
	"github.com/stretchr/testify/assert"
)

func TestMaintainerService_GetMaintainers(t *testing.T) {
	t.Run("returns parsed maintainers", func(t *testing.T) {
		gpg := &mockGPGVerifier{
			listKeysWithColonsFn: func() (string, error) {
				return "pub:u:4096:1:AABBCCDDAABBCCDD:1600000000:::-:::\n" +
					"uid:u::::::::John Doe <john@example.com>:\n", nil
			},
		}
		svc := NewMaintainerService(gpg)
		result := svc.GetMaintainers()
		assert.Len(t, result, 1)
		assert.Equal(t, "AABBCCDDAABBCCDD", result[0].KeyID)
		assert.Equal(t, "John Doe", result[0].Name)
		assert.Equal(t, "john@example.com", result[0].Email)
	})

	t.Run("returns empty on GPG error", func(t *testing.T) {
		gpg := &mockGPGVerifier{
			listKeysWithColonsFn: func() (string, error) {
				return "", errors.New("gpg not available")
			},
		}
		svc := NewMaintainerService(gpg)
		result := svc.GetMaintainers()
		assert.Empty(t, result)
	})
}

func TestMaintainerService_ListMaintainersRaw(t *testing.T) {
	gpg := &mockGPGVerifier{
		listKeysFn: func() (string, error) {
			return "raw output", nil
		},
	}
	svc := NewMaintainerService(gpg)
	raw, err := svc.ListMaintainersRaw()
	assert.NoError(t, err)
	assert.Equal(t, "raw output", raw)
}

func TestParseGPGKeys(t *testing.T) {
	t.Run("single key with uid", func(t *testing.T) {
		output := "pub:u:4096:1:AABBCCDDAABBCCDD:1600000000:::-:::\n" +
			"uid:u::::::::John Doe <john@example.com>:\n"
		result := parseGPGKeys(output)
		assert.Equal(t, []domain.Maintainer{
			{KeyID: "AABBCCDDAABBCCDD", Name: "John Doe", Email: "john@example.com"},
		}, result)
	})

	t.Run("multiple keys", func(t *testing.T) {
		output := "pub:u:4096:1:1111111111111111:1600000000:::-:::\n" +
			"uid:u::::::::Alice <alice@example.com>:\n" +
			"pub:u:4096:1:2222222222222222:1600000000:::-:::\n" +
			"uid:u::::::::Bob <bob@example.com>:\n"
		result := parseGPGKeys(output)
		assert.Len(t, result, 2)
		assert.Equal(t, "1111111111111111", result[0].KeyID)
		assert.Equal(t, "Alice", result[0].Name)
		assert.Equal(t, "2222222222222222", result[1].KeyID)
		assert.Equal(t, "Bob", result[1].Name)
	})

	t.Run("uid without email brackets", func(t *testing.T) {
		output := "pub:u:4096:1:AABBCCDDAABBCCDD:1600000000:::-:::\n" +
			"uid:u::::::::Just A Name:\n"
		result := parseGPGKeys(output)
		assert.Len(t, result, 1)
		assert.Equal(t, "Just A Name", result[0].Name)
		assert.Equal(t, "", result[0].Email)
	})

	t.Run("empty output", func(t *testing.T) {
		result := parseGPGKeys("")
		assert.Empty(t, result)
	})

	t.Run("pub without uid", func(t *testing.T) {
		output := "pub:u:4096:1:AABBCCDDAABBCCDD:1600000000:::-:::\n"
		result := parseGPGKeys(output)
		assert.Len(t, result, 1)
		assert.Equal(t, "AABBCCDDAABBCCDD", result[0].KeyID)
		assert.Equal(t, "", result[0].Name)
	})

	t.Run("short key id extracted from last 16 chars", func(t *testing.T) {
		output := "pub:u:4096:1:0123456789ABCDEF0123456789ABCDEF:1600000000:::-:::\n" +
			"uid:u::::::::Test <test@test.com>:\n"
		result := parseGPGKeys(output)
		assert.Len(t, result, 1)
		assert.Equal(t, "0123456789ABCDEF", result[0].KeyID)
	})

	t.Run("key id too short is empty", func(t *testing.T) {
		output := "pub:u:4096:1:SHORT:1600000000:::-:::\n" +
			"uid:u::::::::Test <test@test.com>:\n"
		result := parseGPGKeys(output)
		assert.Len(t, result, 1)
		assert.Equal(t, "", result[0].KeyID)
	})

	t.Run("uid with too few fields is skipped", func(t *testing.T) {
		output := "pub:u:4096:1:AABBCCDDAABBCCDD:1600000000:::-:::\n" +
			"uid:short\n"
		result := parseGPGKeys(output)
		assert.Len(t, result, 1)
		assert.Equal(t, "", result[0].Name)
	})

	t.Run("lines with fewer than 2 fields skipped", func(t *testing.T) {
		output := "garbage\npub:u:4096:1:AABBCCDDAABBCCDD:1600000000:::-:::\n"
		result := parseGPGKeys(output)
		assert.Len(t, result, 1)
	})
}
