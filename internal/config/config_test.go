package config

import (
	"testing"
)

func TestBaseURLNormalization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Empty BaseURL",
			input:    "",
			expected: "",
		},
		{
			name:     "Single Slash",
			input:    "/",
			expected: "",
		},
		{
			name:     "Path without leading slash",
			input:    "irgsh",
			expected: "/irgsh",
		},
		{
			name:     "Path with trailing slash",
			input:    "/irgsh/",
			expected: "/irgsh",
		},
		{
			name:     "Path without leading and with trailing slash",
			input:    "irgsh/",
			expected: "/irgsh",
		},
		{
			name:     "Properly formatted path",
			input:    "/irgsh",
			expected: "/irgsh",
		},
		{
			name:     "Nested path",
			input:    "/api/irgsh/",
			expected: "/api/irgsh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &IrgshConfig{
				Chief: ChiefConfig{
					BaseURL: tt.input,
				},
				// Minimal required fields to pass validator
				Monitoring: MonitoringConfig{
					HeartbeatInterval: 30,
				},
			}

			// We need to call applyDefaults to trigger normalization
			// Note: applyDefaults also runs validator, so we need some basic fields
			_ = applyDefaults(cfg)

			if cfg.Chief.BaseURL != tt.expected {
				t.Errorf("expected BaseURL %q, got %q", tt.expected, cfg.Chief.BaseURL)
			}
		})
	}
}
