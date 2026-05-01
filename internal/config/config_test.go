package config

import (
	"testing"
)

func TestNormalizeChiefConfig(t *testing.T) {
	tests := []struct {
		name      string
		input     ChiefConfig
		wantAddr  string
		wantBase  string
		wantPublic string
	}{
		{
			name: "trailing slashes",
			input: ChiefConfig{
				Address:   "http://localhost:8080/",
				BaseURL:   "/irgsh/",
				PublicURL: "https://irgsh.id/",
			},
			wantAddr:  "http://localhost:8080",
			wantBase:  "/irgsh",
			wantPublic: "https://irgsh.id",
		},
		{
			name: "missing leading slash on base_url",
			input: ChiefConfig{
				Address:   "http://localhost:8080",
				BaseURL:   "irgsh",
				PublicURL: "https://irgsh.id",
			},
			wantAddr:  "http://localhost:8080",
			wantBase:  "/irgsh",
			wantPublic: "https://irgsh.id",
		},
		{
			name: "root base_url",
			input: ChiefConfig{
				Address:   "http://localhost:8080",
				BaseURL:   "/",
				PublicURL: "https://irgsh.id",
			},
			wantAddr:  "http://localhost:8080",
			wantBase:  "",
			wantPublic: "https://irgsh.id",
		},
		{
			name: "empty values",
			input: ChiefConfig{
				Address:   "http://localhost:8080",
				BaseURL:   "",
				PublicURL: "",
			},
			wantAddr:  "http://localhost:8080",
			wantBase:  "",
			wantPublic: "",
		},
		{
			name: "nested base_url",
			input: ChiefConfig{
				Address:   "http://localhost:8080",
				BaseURL:   "/api/v1/",
				PublicURL: "https://irgsh.id",
			},
			wantAddr:  "http://localhost:8080",
			wantBase:  "/api/v1",
			wantPublic: "https://irgsh.id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.input
			normalizeChiefConfig(&cfg)
			if cfg.Address != tt.wantAddr {
				t.Errorf("normalizeChiefConfig() Address = %v, want %v", cfg.Address, tt.wantAddr)
			}
			if cfg.BaseURL != tt.wantBase {
				t.Errorf("normalizeChiefConfig() BaseURL = %v, want %v", cfg.BaseURL, tt.wantBase)
			}
			if cfg.PublicURL != tt.wantPublic {
				t.Errorf("normalizeChiefConfig() PublicURL = %v, want %v", cfg.PublicURL, tt.wantPublic)
			}
		})
	}
}
