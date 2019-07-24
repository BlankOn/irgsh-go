package main

import (
	"errors"
	"fmt"
	"github.com/hpcloud/tail"
	"log"
	"os"
	"os/exec"
	"strings"
)

type IrgshConfig struct {
	Redis   string        `json:"redis"`
	Chief   ChiefConfig   `json:"chief"`
	Builder BuilderConfig `json:"builder"`
	ISO     ISOConfig     `json:"iso"`
	Repo    RepoConfig    `json:"repo"`
	IsTest  bool          `json:"is_test"`
}

type ChiefConfig struct {
	Address string `json:"address" validate:"required"`
	Workdir string `json:"workdir" validate:"required"`
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
}

func CmdExec(cmdStr string, cmdDesc string, logPath string) (err error) {
	if len(cmdStr) == 0 {
		return errors.New("No command string provided.")
	}

	if len(logPath) > 0 {

		logPathArr := strings.Split(logPath, "/")
		logPathArr = logPathArr[:len(logPathArr)-1]
		logDir := "/" + strings.Join(logPathArr, "/")
		os.MkdirAll(logDir, os.ModePerm)
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, _ = f.WriteString("\n")
		if len(cmdDesc) > 0 {
			cmdDescSplitted := strings.Split(cmdDesc, "\n")
			for _, desc := range cmdDescSplitted {
				_, _ = f.WriteString("##### " + desc + "\n")
			}
		}
		_, _ = f.WriteString("##### RUN " + cmdStr + "\n")
		f.Close()
		cmdStr += " 2>&1 | tee -a " + logPath
	}
	// `set -o pipefail` will forces to return the original exit code
	cmd := exec.Command("bash", "-c", "set -o pipefail && "+cmdStr)
	err = cmd.Run()

	return
}

func StreamLog(path string) {
	t, err := tail.TailFile(path, tail.Config{Follow: true})
	if err != nil {
		log.Printf("error: %v\n", err)
	}
	for line := range t.Lines {
		fmt.Println(line.Text)
	}
}
