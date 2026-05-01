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

func TestBaseURLValidation(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{"valid", "/irgsh", false},
		{"valid nested", "/api/v1", false},
		{"valid with hyphen", "/irgsh-go", false},
		{"valid with underscore", "/irgsh_go", false},
		{"empty", "", false},
		{"invalid space", "/irgsh go", true},
		{"invalid query", "/irgsh?a=b", true},
		{"invalid control", "/irgsh\n", true},
		{"invalid protocol", "http://irgsh", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &IrgshConfig{
				Chief: ChiefConfig{
					Address:  "http://localhost:8080",
					Workdir:  "/tmp",
					GnupgDir: "/tmp",
					BaseURL:  tt.baseURL,
				},
				Builder: BuilderConfig{
					Workdir:              "/tmp",
					UpstreamDistCodename: "sid",
					UpstreamDistUrl:      "http://deb.debian.org/debian",
				},
			}
			err := applyDefaults(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("applyDefaults() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
