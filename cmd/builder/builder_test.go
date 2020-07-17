package main

import (
	"log"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/blankon/irgsh-go/internal/config"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	irgshConfig, _ = config.LoadConfig()
	dir, _ := os.Getwd()
	irgshConfig.Builder.Workdir = dir + "/../tmp"

	m.Run()
}

func TestBuilderClone(t *testing.T) {
	id := time.Now().Format("2006-01-02-150405") + "_" + uuid.New().String()
	log.Println(id)
	payload := "{\"taskUUID\":\"" + id + "\",\"timestamp\":\"2019-04-03T07:23:02.826753827-04:00\",\"sourceUrl\":\"https://github.com/BlankOn/bromo-theme.git\",\"packageUrl\":\"https://github.com/BlankOn-packages/bromo-theme.git\"}"
	_, err := Clone(payload)
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}

	cmdStr := "du -s " + irgshConfig.Builder.Workdir + "/" + id + "/source | cut -d '/' -f1 | head -n 1 | sed 's/ //g' | tr -d '\n' | tr -d '\t' "
	cmd := exec.Command("bash", "-c", cmdStr)
	out, _ := cmd.CombinedOutput()
	cmd.Run()
	log.Println(string(out))
	size, err := strconv.Atoi(string(out))
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}
	assert.NotEqual(t, size, int(0))

	cmdStr = "du -s " + irgshConfig.Builder.Workdir + "/" + id + "/package | cut -d '/' -f1 | head -n 1 | sed 's/ //g' | tr -d '\n' | tr -d '\t' "
	cmd = exec.Command("bash", "-c", cmdStr)
	out, _ = cmd.CombinedOutput()
	cmd.Run()
	size, err = strconv.Atoi(string(out))
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}
	assert.NotEqual(t, size, int(0))
}

func TestBuilderCloneInvalidSourceUrl(t *testing.T) {
	id := time.Now().Format("2006-01-02-150405") + "_" + uuid.New().String()
	log.Println(id)
	payload := "{\"taskUUID\":\"" + id + "\",\"timestamp\":\"2019-04-03T07:23:02.826753827-04:00\",\"sourceUrl\":\"https://github.com/BlankOn/bromo-theme-xyz.git\",\"packageUrl\":\"https://github.com/BlankOn-packages/bromo-theme.git\"}"
	_, err := Clone(payload)
	assert.Equal(t, err != nil, true, "Should be error")

	cmdStr := "du -s " + irgshConfig.Builder.Workdir + "/" + id + "/source | cut -d '/' -f1 | head -n 1 | sed 's/ //g' | tr -d '\n' | tr -d '\t' "
	cmd := exec.Command("bash", "-c", cmdStr)
	out, _ := cmd.CombinedOutput()
	cmd.Run()
	size, err := strconv.Atoi(string(out))
	assert.Equal(t, err != nil, true, "Should be error")
	assert.Equal(t, size, int(0))

	cmdStr = "du -s " + irgshConfig.Builder.Workdir + "/" + id + "/package | cut -d '/' -f1 | head -n 1 | sed 's/ //g' | tr -d '\n' | tr -d '\t' "
	cmd = exec.Command("bash", "-c", cmdStr)
	out, _ = cmd.CombinedOutput()
	cmd.Run()
	size, err = strconv.Atoi(string(out))
	assert.Equal(t, err != nil, true, "Should be error")
	assert.Equal(t, size, int(0))
}

func TestBuilderCloneInvalidPackadeUrl(t *testing.T) {
	id := time.Now().Format("2006-01-02-150405") + "_" + uuid.New().String()
	log.Println(id)
	payload := "{\"taskUUID\":\"" + id + "\",\"timestamp\":\"2019-04-03T07:23:02.826753827-04:00\",\"sourceUrl\":\"https://github.com/BlankOn/bromo-theme.git\",\"packageUrl\":\"https://github.com/BlankOn-packages/bromo-theme-xyz.git\"}"
	_, err := Clone(payload)
	assert.Equal(t, err != nil, true, "Should be error")

	cmdStr := "du -s " + irgshConfig.Builder.Workdir + "/" + id + "/source | cut -d '/' -f1 | head -n 1 | sed 's/ //g' | tr -d '\n' | tr -d '\t' "
	cmd := exec.Command("bash", "-c", cmdStr)
	out, _ := cmd.CombinedOutput()
	cmd.Run()
	size, err := strconv.Atoi(string(out))
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}
	assert.NotEqual(t, size, int(0))

	cmdStr = "du -s " + irgshConfig.Builder.Workdir + "/" + id + "/package | cut -d '/' -f1 | head -n 1 | sed 's/ //g' | tr -d '\n' | tr -d '\t' "
	cmd = exec.Command("bash", "-c", cmdStr)
	out, _ = cmd.CombinedOutput()
	cmd.Run()
	size, err = strconv.Atoi(string(out))
	assert.Equal(t, err != nil, true, "Should be error")
	assert.Equal(t, size, int(0))
}

// This tests below need pbuilder/sudo

func TestBuilderBuildPreparation(t *testing.T) {
	t.Skip()
}

func TestBuilderBuildPackage(t *testing.T) {
	t.Skip()
}

func TestBuilderStorePackage(t *testing.T) {
	t.Skip()
}
