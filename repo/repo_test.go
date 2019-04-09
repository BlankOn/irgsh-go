package main

import (
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	validator "gopkg.in/go-playground/validator.v9"
)

func TestRepoPreparation(t *testing.T) {
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
	err = validate.Struct(irgshConfig.Repo)
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}
	dir, _ := os.Getwd()
	irgshConfig.Repo.Workdir = dir + "/../tmp"
	irgshConfig.IsTest = true
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
