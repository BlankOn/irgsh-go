package main

import (
	"log"
	"os/exec"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaseInitBase(t *testing.T) {
	t.Skip() // This still need sudo
	err := InitBase()
	if err != nil {
		log.Println(err.Error())
		assert.Equal(t, true, false, "Should not reach here")
	}

	cmdStr := "du -s /var/cache/pbuilder/base.tgz | "
	cmdStr += "cut -d '/' -f1 | head -n 1 | sed 's/ //g' | "
	cmdStr += "tr -d '\n' | tr -d '\t' "
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
