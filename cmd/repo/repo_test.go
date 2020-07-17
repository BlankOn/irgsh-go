package main

import (
	"log"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/blankon/irgsh-go/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	irgshConfig, _ = config.LoadConfig()
	dir, _ := os.Getwd()
	irgshConfig.Builder.Workdir = dir + "/../tmp"

	m.Run()
}

func TestBaseInitRepo(t *testing.T) {
	err := InitRepo()
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}

	cmdStr := "du -s " + irgshConfig.Repo.Workdir + "/verbeek | cut -d '/' -f1 | head -n 1 | sed 's/ //g' | tr -d '\n' | tr -d '\t' "
	cmd := exec.Command("bash", "-c", cmdStr)
	out, _ := cmd.CombinedOutput()
	cmd.Run()
	size, err := strconv.Atoi(string(out))
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}
	assert.NotEqual(t, size, int(0))
}

func TestBaseInitRepoConfigCheck(t *testing.T) {
	t.Skip()
}
