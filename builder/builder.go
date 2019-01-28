package main

import (
	//"errors"
	"encoding/json"
	"fmt"
	"gopkg.in/src-d/go-git.v4"
	"os"
	"time"
)

func Clone(payload string) (string, error) {
	fmt.Println("Payload :")
	fmt.Println(payload)
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	// Cloning source file
	sourceURL := raw["sourceUrl"].(string)
	_, err := git.PlainClone(workdir+"/"+raw["taskUUID"].(string)+"/source", false, &git.CloneOptions{
		URL:      sourceURL,
		Progress: os.Stdout,
	})
	if err != nil {
		fmt.Println(err.Error())
	}

	// Cloning Debian package file
	packageURL := raw["packageUrl"].(string)
	_, err = git.PlainClone(workdir+"/"+raw["taskUUID"].(string)+"/package", false, &git.CloneOptions{
		URL:      packageURL,
		Progress: os.Stdout,
	})
	if err != nil {
		fmt.Println(err.Error())
	}
	time.Sleep(0 * time.Second)
	return payload, nil
}
