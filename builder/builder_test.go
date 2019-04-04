package main

import (
	"io/ioutil"
	"log"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/ghodss/yaml"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	validator "gopkg.in/go-playground/validator.v9"
)

func TestPreparation(t *testing.T) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	configPath = "../utils/config.yml"
	irgshConfig = IrgshConfig{}
	yamlFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}
	err = yaml.Unmarshal(yamlFile, &irgshConfig)
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}
	validate := validator.New()
	err = validate.Struct(irgshConfig.Builder)
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}
	irgshConfig.Builder.Workdir = "/tmp/"
}

func TestClone(t *testing.T) {
	id := time.Now().Format("2006-01-02-150405") + "_" + uuid.New().String()
	log.Println(id)
	payload := "{\"taskUUID\":\"" + id + "\",\"timestamp\":\"2019-04-03T07:23:02.826753827-04:00\",\"sourceUrl\":\"https://github.com/BlankOn/bromo-theme.git\",\"packageUrl\":\"https://github.com/BlankOn-packages/bromo-theme.git\"}"
	_, err := Clone(payload)
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}

	cmdStr := "du -s /tmp/" + id + "/source | cut -d '/' -f1 | head -n 1 | sed 's/ //g' | tr -d '\n' | tr -d '\t' "
	cmd := exec.Command("bash", "-c", cmdStr)
	out, _ := cmd.CombinedOutput()
	cmd.Run()
	size, err := strconv.Atoi(string(out))
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}
	assert.NotEqual(t, size, int(0))

	cmdStr = "du -s /tmp/" + id + "/package | cut -d '/' -f1 | head -n 1 | sed 's/ //g' | tr -d '\n' | tr -d '\t' "
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

func TestCloneInvalidSourceUrl(t *testing.T) {
	id := time.Now().Format("2006-01-02-150405") + "_" + uuid.New().String()
	log.Println(id)
	payload := "{\"taskUUID\":\"" + id + "\",\"timestamp\":\"2019-04-03T07:23:02.826753827-04:00\",\"sourceUrl\":\"https://github.com/BlankOn/bromo-theme-xyz.git\",\"packageUrl\":\"https://github.com/BlankOn-packages/bromo-theme.git\"}"
	_, err := Clone(payload)
	assert.Equal(t, err != nil, true, "Should be error")

	cmdStr := "du -s /tmp/" + id + "/source | cut -d '/' -f1 | head -n 1 | sed 's/ //g' | tr -d '\n' | tr -d '\t' "
	cmd := exec.Command("bash", "-c", cmdStr)
	out, _ := cmd.CombinedOutput()
	cmd.Run()
	size, err := strconv.Atoi(string(out))
	assert.Equal(t, err != nil, true, "Should be error")
	assert.Equal(t, size, int(0))

	cmdStr = "du -s /tmp/" + id + "/package | cut -d '/' -f1 | head -n 1 | sed 's/ //g' | tr -d '\n' | tr -d '\t' "
	cmd = exec.Command("bash", "-c", cmdStr)
	out, _ = cmd.CombinedOutput()
	cmd.Run()
	size, err = strconv.Atoi(string(out))
	assert.Equal(t, err != nil, true, "Should be error")
	assert.Equal(t, size, int(0))
}

func TestCloneInvalidPackadeUrl(t *testing.T) {
	id := time.Now().Format("2006-01-02-150405") + "_" + uuid.New().String()
	log.Println(id)
	payload := "{\"taskUUID\":\"" + id + "\",\"timestamp\":\"2019-04-03T07:23:02.826753827-04:00\",\"sourceUrl\":\"https://github.com/BlankOn/bromo-theme.git\",\"packageUrl\":\"https://github.com/BlankOn-packages/bromo-theme-xyz.git\"}"
	_, err := Clone(payload)
	assert.Equal(t, err != nil, true, "Should be error")

	cmdStr := "du -s /tmp/" + id + "/source | cut -d '/' -f1 | head -n 1 | sed 's/ //g' | tr -d '\n' | tr -d '\t' "
	cmd := exec.Command("bash", "-c", cmdStr)
	out, _ := cmd.CombinedOutput()
	cmd.Run()
	size, err := strconv.Atoi(string(out))
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}
	assert.NotEqual(t, size, int(0))

	cmdStr = "du -s /tmp/" + id + "/package | cut -d '/' -f1 | head -n 1 | sed 's/ //g' | tr -d '\n' | tr -d '\t' "
	cmd = exec.Command("bash", "-c", cmdStr)
	out, _ = cmd.CombinedOutput()
	cmd.Run()
	size, err = strconv.Atoi(string(out))
	assert.Equal(t, err != nil, true, "Should be error")
	assert.Equal(t, size, int(0))
}
