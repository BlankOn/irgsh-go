package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeIDPattern(t *testing.T) {
	valid := []string{
		"abc123",
		"my-package",
		"my_package",
		"my.package",
		"version+1",
		"2024-01-01-120000_abc123_FINGERPRINT_pkg",
		"a",
		"A",
		"0",
		"a.b-c_d+e",
	}
	for _, id := range valid {
		t.Run("valid/"+id, func(t *testing.T) {
			assert.True(t, SafeIDPattern.MatchString(id), "expected %q to match", id)
		})
	}

	invalid := []string{
		"",
		"foo bar",
		"foo/bar",
		"../etc/passwd",
		"id;rm -rf",
		"id\nbar",
		"id\tbar",
		"foo<bar",
		"foo>bar",
		"foo|bar",
		"foo&bar",
		"foo`bar`",
		"foo$bar",
		"foo(bar)",
		"foo{bar}",
	}
	for _, id := range invalid {
		name := id
		if name == "" {
			name = "empty"
		}
		t.Run("invalid/"+name, func(t *testing.T) {
			assert.False(t, SafeIDPattern.MatchString(id), "expected %q to NOT match", id)
		})
	}
}

func TestValidateID(t *testing.T) {
	t.Run("valid id returns nil", func(t *testing.T) {
		err := ValidateID("valid-id_123", "pipeline")
		require.NoError(t, err)
	})

	t.Run("invalid id returns descriptive error", func(t *testing.T) {
		err := ValidateID("bad/id", "pipeline")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid pipeline")
		assert.Contains(t, err.Error(), "bad/id")
	})

	t.Run("empty id returns error", func(t *testing.T) {
		err := ValidateID("", "artifact")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid artifact")
	})
}
