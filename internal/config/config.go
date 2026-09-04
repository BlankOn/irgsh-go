package config

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ghodss/yaml"
	validator "gopkg.in/go-playground/validator.v9"
)

type IrgshConfig struct {
	Redis        string             `json:"redis" validate:"required"`
	Chief        ChiefConfig        `json:"chief"`
	Builder      BuilderConfig      `json:"builder"`
	ISO          ISOConfig          `json:"iso"`
	Repo         RepoConfig         `json:"repo"`
	Monitoring   MonitoringConfig   `json:"monitoring"`
	Notification NotificationConfig `json:"notification"`
	Storage      StorageConfig      `json:"storage"`
	IsTest       bool               `json:"is_test"`
	IsDev        bool               `json:"is_dev"`
}

type ChiefConfig struct {
	Address  string `json:"address" validate:"required"`
	Workdir  string `json:"workdir" validate:"required"`
	GnupgDir string `json:"gnupg_dir" validate:"required"` // GNUPG dir path
}

type BuilderConfig struct {
	Workdir              string `json:"workdir" validate:"required"`
	DistCodename         string `json:"dist_codename" validate:"required"`          // verbeek - the distribution this builder builds for, also its queue identity
	UpstreamDistCodename string `json:"upstream_dist_codename" validate:"required"` // sid
	UpstreamDistUrl      string `json:"upstream_dist_url" validate:"required"`      // http://kartolo.sby.datautama.net.id/debian
}

type ISOConfig struct {
	// Workdir is the persistent live-build tree the build script runs in.
	// chroot/, cache/, auto/ and local/ are reused between builds unless the
	// submission asks for a cacheless build.
	Workdir      string `json:"workdir" validate:"required"`
	Outputdir    string `json:"outputdir" validate:"required"`
	DistCodename string `json:"dist_codename" validate:"required"` // verbeek - the distribution this ISO builder builds for, also its queue identity
	// RepoURL is the live-build git repository this worker builds from, e.g.
	// https://github.com/BlankOn/blankon-live-build.git. It belongs to the
	// worker rather than the submission: a client only names the branch.
	RepoURL string `json:"repo_url" validate:"required"`
	// PublicBaseURL is where the built images are published for users, e.g.
	// http://arsip-dev.blankonlinux.id/iso. The build script needs it for the
	// zsync control file and its announcements.
	PublicBaseURL string `json:"public_base_url"`
	// TelegramBotKey is optional. When empty the build script skips its
	// Telegram announcement.
	TelegramBotKey string `json:"telegram_bot_key"`
}

type RepoConfig struct {
	Workdir                    string `json:"workdir" validate:"required"`
	DistName                   string `json:"dist_name"`                            // BlankOn
	DistLabel                  string `json:"dist_label"`                           // BlankOn
	DistCodename               string `json:"dist_codename" validate:"required"`    // verbeek - also this repo's queue identity
	DistComponents             string `json:"dist_components" validate:"required"`  // main restricted extras extras-restricted
	DistSupportedArchitectures string `json:"dist_supported_architectures"`         // amd64 source
	DistVersion                string `json:"dist_version"`                         // 12.0
	DistVersionDesc            string `json:"dist_version_desc"`                    // BlankOn Linux 12.0 Verbeek
	DistSigningKey             string `json:"dist_signing_key" validate:"required"` // 55BD65A0B3DA3A59ACA60932E2FE388D53B56A71
	UpstreamName               string `json:"upstream_name"`                        // merge.sid
	UpstreamDistCodename       string `json:"upstream_dist_codename"`               // sid
	UpstreamDistUrl            string `json:"upstream_dist_url"`                    // http://kartolo.sby.datautama.net.id/debian
	UpstreamDistComponents     string `json:"upstream_dist_components"`             // main non-free>restricted contrib>extras
	// PublicURL is where this repository is published for users to install
	// from, e.g. http://arsip-dev.blankonlinux.id/dev. Clients need it to
	// check a package against the repository it is actually going into.
	PublicURL string `json:"public_url"`
	GnupgDir  string `json:"gnupg_dir" validate:"required"` // GNUPG dir path
}

type MonitoringConfig struct {
	Enabled           bool `json:"enabled"`            // Enable/disable monitoring
	HeartbeatInterval int  `json:"heartbeat_interval"` // Worker heartbeat frequency in seconds (default: 30)
	InstanceTimeout   int  `json:"instance_timeout"`   // Mark offline after this duration in seconds (default: 90)
	CleanupInterval   int  `json:"cleanup_interval"`   // Cleanup check frequency in seconds (default: 3600). Instances removed after 24h of no heartbeat.
}

type NotificationConfig struct {
	WebhookURL string `json:"webhook_url"` // Webhook URL for job notifications
}

type StorageConfig struct {
	DatabasePath string `json:"database_path"` // Path to SQLite database file (default: /var/lib/irgsh/chief/irgsh.db)
	MaxJobs      int    `json:"max_jobs"`      // Maximum number of jobs to retain (default: 1000)
	MaxISOJobs   int    `json:"max_iso_jobs"`  // Maximum number of ISO jobs to retain (default: 200)
	// MaxImportJobs is the maximum number of package import jobs to retain
	// (default: 200)
	MaxImportJobs int `json:"max_import_jobs"`
}

// Component identifies which part of irgsh is loading the config, so that
// only its own section (plus Redis, always mandatory) is required to be
// present. Values must match the corresponding IrgshConfig field name, since
// they're passed straight to validator's StructPartial.
type Component string

const (
	ComponentChief   Component = "Chief"
	ComponentBuilder Component = "Builder"
	ComponentRepo    Component = "Repo"
	ComponentISO     Component = "ISO"
)

// DistQueue returns the Machinery queue name a given distribution's
// build/repo/iso tasks are routed through.
func DistQueue(dist string) string {
	return "irgsh-" + dist
}

// LoadConfigFromPath loads irgsh config from a specific file path
func LoadConfigFromPath(configPath string, component Component) (cfg IrgshConfig, err error) {
	if configPath == "" {
		err = fmt.Errorf("config path is required")
		return
	}

	yamlFile, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	log.Println("load config from : ", configPath)

	err = yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		return
	}

	return cfg, applyDefaults(&cfg, component)
}

// LoadConfig load irgsh config from file
func LoadConfig(component Component) (cfg IrgshConfig, err error) {
	configPaths := []string{
		"/etc/irgsh/config.yaml",
		"../../utils/config.yaml",
		"./utils/config.yaml",
	}
	configPath := os.Getenv("IRGSH_CONFIG_PATH")
	isDev := os.Getenv("DEV") == "1"
	yamlFile, err := os.ReadFile(configPath)
	if err != nil {
		// load from predefined configPaths when no IRGSH_CONFIG_PATH set
		for _, p := range configPaths {
			yamlFile, err = os.ReadFile(p)
			if err == nil {
				log.Println("load config from : ", p)
				break
			}
		}
		if err != nil {
			return
		}
	}
	if isDev {
		yamlFile, err = os.ReadFile("./utils/config.yaml")
		if err != nil {
			return
		}
	}

	err = yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		return
	}

	return cfg, applyDefaults(&cfg, component)
}

func applyDefaults(cfg *IrgshConfig, component Component) error {
	if cfg.Storage.DatabasePath == "" {
		cfg.Storage.DatabasePath = "/var/lib/irgsh/chief/irgsh.db"
	}
	if cfg.Storage.MaxJobs == 0 {
		cfg.Storage.MaxJobs = 1000
	}
	if cfg.Storage.MaxISOJobs == 0 {
		cfg.Storage.MaxISOJobs = 200
	}
	if cfg.Storage.MaxImportJobs == 0 {
		cfg.Storage.MaxImportJobs = 200
	}

	isDev := os.Getenv("DEV") == "1"
	if isDev {
		cwd, _ := os.Getwd()
		tmpDir := cwd + "/tmp/"
		if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
			os.Mkdir(tmpDir, 0755)
		}
		cfg.Chief.Workdir = strings.ReplaceAll(cfg.Chief.Workdir, "/var/lib/", tmpDir)
		cfg.Builder.Workdir = strings.ReplaceAll(cfg.Builder.Workdir, "/var/lib/", tmpDir)
		cfg.Repo.Workdir = strings.ReplaceAll(cfg.Repo.Workdir, "/var/lib/", tmpDir)
		cfg.ISO.Workdir = strings.ReplaceAll(cfg.ISO.Workdir, "/var/lib/", tmpDir)
		cfg.Storage.DatabasePath = strings.ReplaceAll(cfg.Storage.DatabasePath, "/var/lib/", tmpDir)
	}
	cfg.IsDev = isDev

	if cfg.Monitoring.HeartbeatInterval == 0 {
		cfg.Monitoring.HeartbeatInterval = 30
	}
	if cfg.Monitoring.InstanceTimeout == 0 {
		cfg.Monitoring.InstanceTimeout = 90
	}
	if cfg.Monitoring.CleanupInterval == 0 {
		cfg.Monitoring.CleanupInterval = 3600
	}

	validate := validator.New()
	return validate.StructPartial(cfg, "Redis", string(component))
}
