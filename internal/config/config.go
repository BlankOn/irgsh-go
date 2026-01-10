package config

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/ghodss/yaml"
	validator "gopkg.in/go-playground/validator.v9"
)

type IrgshConfig struct {
	Redis        string             `json:"redis"`
	Chief        ChiefConfig        `json:"chief"`
	Builder      BuilderConfig      `json:"builder"`
	ISO          ISOConfig          `json:"iso"`
	Repo         RepoConfig         `json:"repo"`
	Monitoring   MonitoringConfig   `json:"monitoring"`
	Notification NotificationConfig `json:"notification"`
	IsTest       bool               `json:"is_test"`
	IsDev        bool               `json:"is_dev"`
}

type ChiefConfig struct {
	Address  string `json:"address" validate:"required"`
	Workdir  string `json:"workdir" validate:"required"`
	GnupgDir string `json:"gnupg_dir" validate:"required"` // GNUPG dir path
}

type BuilderConfig struct {
	Workdir         string `json:"workdir" validate:"required"`
	UpstreamDistUrl string `json:"upstream_dist_url" validate:"required"` // http://kartolo.sby.datautama.net.id/debian
}

type ISOConfig struct {
	Workdir string `json:"workdir" validate:"required"`
}

type RepoConfig struct {
	Workdir                    string `json:"workdir" validate:"required"`
	DistName                   string `json:"dist_name" validate:"required"`                    // BlankOn
	DistLabel                  string `json:"dist_label" validate:"required"`                   // BlankOn
	DistCodename               string `json:"dist_codename" validate:"required"`                // verbeek
	DistComponents             string `json:"dist_components" validate:"required"`              // main restricted extras extras-restricted
	DistSupportedArchitectures string `json:"dist_supported_architectures" validate:"required"` // amd64 source
	DistVersion                string `json:"dist_version" validate:"required"`                 // 12.0
	DistVersionDesc            string `json:"dist_version_desc" validate:"required"`            // BlankOn Linux 12.0 Verbeek
	DistSigningKey             string `json:"dist_signing_key" validate:"required"`             // 55BD65A0B3DA3A59ACA60932E2FE388D53B56A71
	UpstreamName               string `json:"upstream_name" validate:"required"`                // merge.sid
	UpstreamDistCodename       string `json:"upstream_dist_codename" validate:"required"`       // sid
	UpstreamDistUrl            string `json:"upstream_dist_url" validate:"required"`            // http://kartolo.sby.datautama.net.id/debian
	UpstreamDistComponents     string `json:"upstream_dist_components" validate:"required"`     // main non-free>restricted contrib>extras
	GnupgDir                   string `json:"gnupg_dir" validate:"required"`                    // GNUPG dir path
}

type MonitoringConfig struct {
	Enabled           bool `json:"enabled"`             // Enable/disable monitoring
	HeartbeatInterval int  `json:"heartbeat_interval"`  // Worker heartbeat frequency in seconds (default: 30)
	InstanceTimeout   int  `json:"instance_timeout"`    // Mark offline after this duration in seconds (default: 90)
	CleanupInterval   int  `json:"cleanup_interval"`    // Cleanup check frequency in seconds (default: 3600). Instances removed after 24h of no heartbeat.
}

type NotificationConfig struct {
	WebhookURL string `json:"webhook_url"` // Webhook URL for job notifications
}

// LoadConfigFromPath loads irgsh config from a specific file path
func LoadConfigFromPath(configPath string) (config IrgshConfig, err error) {
	if configPath == "" {
		err = fmt.Errorf("config path is required")
		return
	}

	yamlFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		return
	}
	log.Println("load config from : ", configPath)

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return
	}

	isDev := os.Getenv("DEV") == "1"
	if isDev {
		// Since it's in dev env, let's move some path to ./tmp
		cwd, _ := os.Getwd()
		tmpDir := cwd + "/tmp/"
		if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
			os.Mkdir(tmpDir, 0755)
		}
		config.Chief.Workdir = strings.ReplaceAll(config.Chief.Workdir, "/var/lib/", tmpDir)
		config.Builder.Workdir = strings.ReplaceAll(config.Builder.Workdir, "/var/lib/", tmpDir)
		config.Repo.Workdir = strings.ReplaceAll(config.Repo.Workdir, "/var/lib/", tmpDir)
	}
	config.IsDev = isDev

	// Set monitoring defaults
	if config.Monitoring.HeartbeatInterval == 0 {
		config.Monitoring.HeartbeatInterval = 30
	}
	if config.Monitoring.InstanceTimeout == 0 {
		config.Monitoring.InstanceTimeout = 90
	}
	if config.Monitoring.CleanupInterval == 0 {
		config.Monitoring.CleanupInterval = 3600 // 1 hour
	}

	validate := validator.New()
	err = validate.Struct(config)

	return
}

// LoadConfig load irgsh config from file
func LoadConfig() (config IrgshConfig, err error) {
	configPaths := []string{
		"/etc/irgsh/config.yaml",
		"../../utils/config.yaml",
		"./utils/config.yaml",
	}
	configPath := os.Getenv("IRGSH_CONFIG_PATH")
	isDev := os.Getenv("DEV") == "1"
	yamlFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		// load from predefined configPaths when no IRGSH_CONFIG_PATH set
		for _, config := range configPaths {
			yamlFile, err = ioutil.ReadFile(config)
			if err == nil {
				log.Println("load config from : ", config)
				break
			}
		}
		if err != nil {
			return
		}
	}
	if isDev {
		yamlFile, err = ioutil.ReadFile("./utils/config.yaml")
		if err != nil {
			return
		}
	}

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return
	}

	if isDev {
		// Since it's in dev env, let's move some path to ./tmp
		cwd, _ := os.Getwd()
		tmpDir := cwd + "/tmp/"
		if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
			os.Mkdir(tmpDir, 0755)
		}
		config.Chief.Workdir = strings.ReplaceAll(config.Chief.Workdir, "/var/lib/", tmpDir)
		config.Builder.Workdir = strings.ReplaceAll(config.Builder.Workdir, "/var/lib/", tmpDir)
		config.Repo.Workdir = strings.ReplaceAll(config.Repo.Workdir, "/var/lib/", tmpDir)
	}
	config.IsDev = isDev

	// Set monitoring defaults
	if config.Monitoring.HeartbeatInterval == 0 {
		config.Monitoring.HeartbeatInterval = 30
	}
	if config.Monitoring.InstanceTimeout == 0 {
		config.Monitoring.InstanceTimeout = 90
	}
	if config.Monitoring.CleanupInterval == 0 {
		config.Monitoring.CleanupInterval = 3600 // 1 hour
	}

	validate := validator.New()
	err = validate.Struct(config)

	return
}
