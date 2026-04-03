//go:build integration

package main

import (
	"log"
	"os"
	"testing"

	"github.com/blankon/irgsh-go/internal/config"
)

func TestMain(m *testing.M) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	irgshConfig, _ = config.LoadConfig()
	dir, _ := os.Getwd()
	irgshConfig.Builder.Workdir = dir + "/../tmp"

	m.Run()
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
