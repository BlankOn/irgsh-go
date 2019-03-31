package main

import (
	"encoding/json"
	"fmt"
	"gopkg.in/src-d/go-git.v4"
	"log"
	"os"
	"time"
)

// Main wrapper
func Build(payload string) (next string, err error) {
	fmt.Println("Payload :")
	fmt.Println(payload)
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	logPath := workdir + "/" + raw["taskUUID"].(string) + "/build.log"
	go StreamLog(logPath)

	next, err = Clone(payload)
	if err != nil {
		return
	}

	next, err = BuildPreparation(payload)
	if err != nil {
		return
	}

	next, err = BuildPackage(payload)
	if err != nil {
		return
	}

	next, err = StorePackage(payload)

	if err == nil {
		log.Println("[ BUILD DONE ]")
	}

	return
}

func Clone(payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	// Cloning source files
	sourceURL := raw["sourceUrl"].(string)
	_, err = git.PlainClone(workdir+"/"+raw["taskUUID"].(string)+"/source", false, &git.CloneOptions{
		URL:      sourceURL,
		Progress: os.Stdout,
	})
	if err != nil {
		fmt.Println(err.Error())
	}

	// Cloning Debian package files
	packageURL := raw["packageUrl"].(string)
	_, err = git.PlainClone(workdir+"/"+raw["taskUUID"].(string)+"/package", false, &git.CloneOptions{
		URL:      packageURL,
		Progress: os.Stdout,
	})
	if err != nil {
		fmt.Println(err.Error())
	}
	time.Sleep(0 * time.Second)

	next = payload
	return
}

func BuildPreparation(payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	logPath := workdir + "/" + raw["taskUUID"].(string) + "/build.log"

	// Signing DSC
	cmdStr := "cd " + workdir + "/" + raw["taskUUID"].(string) + "/package" + " && debuild -S -k" + signingKey + "  > " + logPath
	err = Execute(cmdStr)
	if err != nil {
		log.Println(cmdStr)
		log.Printf("error: %v\n", err)
		return
	}

	next = payload
	return
}

func BuildPackage(payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	logPath := workdir + "/" + raw["taskUUID"].(string) + "/build.log"

	// Copy the source files
	cmdStr := "cp -vR " + workdir + "/" + raw["taskUUID"].(string) + "/source/* " + workdir + "/" + raw["taskUUID"].(string) + "/package/" + " >> " + logPath
	err = Execute(cmdStr)
	if err != nil {
		log.Println(cmdStr)
		log.Printf("error: %v\n", err)
		return
	}

	// Cleanup pbuilder cache result
	_ = Execute("sudo rm -rf /var/cache/pbuilder/result/*")

	// Building the package
	cmdStr = "cd " + workdir + "/" + raw["taskUUID"].(string) + " && sudo pbuilder build *.dsc >> " + logPath
	err = Execute(cmdStr)
	if err != nil {
		log.Println(cmdStr)
		log.Printf("error: %v\n", err)
		return
	}

	cmdStr = "cp /var/cache/pbuilder/result/* " + workdir + "/" + raw["taskUUID"].(string)
	err = Execute(cmdStr)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	next = payload
	return
}

func StorePackage(payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	logPath := workdir + "/" + raw["taskUUID"].(string) + "/build.log"

	// Building package
	cmdStr := "cd " + workdir + " && tar -zcvf " + raw["taskUUID"].(string) + ".tar.gz " + raw["taskUUID"].(string) + " && curl -v -F 'uploadFile=@" + workdir + "/" + raw["taskUUID"].(string) + ".tar.gz' " + chiefAddress + "/upload?id=" + raw["taskUUID"].(string) + " >> " + logPath
	err = Execute(cmdStr)
	if err != nil {
		log.Println(cmdStr)
		log.Printf("error: %v\n", err)
		return
	}

	next = payload
	return
}
