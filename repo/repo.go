package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

func Repo(payload string) (err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	logPath := workdir + "/artifacts/" + raw["taskUUID"].(string) + "/repo.log"

	cmdStr := "mkdir -p " + workdir + "/artifacts/ && cd " + workdir + "/artifacts/ && wget " + chiefAddress + "/" + raw["taskUUID"].(string) + ".tar.gz && tar -xvf " + raw["taskUUID"].(string) + ".tar.gz"
	log.Println(cmdStr)
	cmd := exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf("cd %s/%s/ && sudo reprepro -v -v -v includedeb %s %s/artifacts/%s/*.deb >>  %s",
		workdir,
		repository.DistCodename,
		repository.DistCodename,
		workdir,
		raw["taskUUID"],
		logPath,
	)
	log.Println(cmdStr)
	cmd = exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	return
}

func InitRepo() (err error) {
	fmt.Println("Initialize repository")

	logPath := workdir + "/init.log"
	go StreamLog(logPath)

	cmdStr := "sudo rm -rf " + workdir + "/" + repository.DistCodename + " && cp -vR ../share/reprepro-template " + workdir + "/" + repository.DistCodename
	cmd := exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		log.Println(cmdStr)
		log.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf("cd %s/%s/conf && cat updates.orig | sed 's/UPSTREAM_NAME/%s/g' | sed 's/UPSTREAM_DIST_CODENAME/%s/g' | sed 's/UPSTREAM_DIST_URL/%s/g' | sed 's/DIST_SUPPORTED_ARCHITECTURES/%s/g' | sed 's/UPSTREAM_DIST_COMPONENTS/%s/g' > updates && rm updates.orig",
		workdir,
		repository.DistCodename,
		repository.UpstreamName,
		repository.UpstreamDistCodename,
		strings.Replace(repository.UpstreamDistUrl, "/", "\\/", -1),
		repository.DistSupportedArchitectures,
		repository.UpstreamDistComponents,
	)
	log.Println(cmdStr)
	cmd = exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf("cd %s/%s/conf && cat distributions.orig | sed 's/DIST_NAME/%s/g' | sed 's/DIST_LABEL/%s/g' | sed 's/DIST_CODENAME/%s/g' | sed 's/DIST_COMPONENTS/%s/g' | sed 's/DIST_SUPPORTED_ARCHITECTURES/%s/g' | sed 's/DIST_VERSION_DESC/%s/g' | sed 's/DIST_VERSION/%s/g' | sed 's/DIST_SIGNING_KEY/%s/g' | sed 's/UPSTREAM_NAME/%s/g' > distributions && rm distributions.orig",
		workdir,
		repository.DistCodename,
		repository.DistName,
		repository.DistLabel,
		repository.DistCodename,
		repository.DistComponents,
		repository.DistSupportedArchitectures,
		repository.DistVersionDesc,
		repository.DistVersion,
		repository.DistSigningKey,
		repository.UpstreamName,
	)
	cmd = exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	repositoryPath := strings.Replace(workdir+"/"+repository.DistCodename, "/", "\\/", -1)
	cmdStr = fmt.Sprintf("cd %s/%s/conf && cat options.orig | sed 's/IRGSH_REPO_WORKDIR/%s/g' > options && rm options.orig",
		workdir,
		repository.DistCodename,
		repositoryPath,
	)
	cmd = exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf("cd %s/%s/ && reprepro -v -v -v export > %s",
		workdir,
		repository.DistCodename,
		logPath,
	)
	cmd = exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	return
}

func UpdateRepo() (err error) {
	fmt.Println("Syncing repository against %s at %s...", repository.UpstreamDistCodename, repository.UpstreamDistUrl)

	logPath := workdir + "/update.log"
	go StreamLog(logPath)

	cmdStr := fmt.Sprintf("cd %s/%s/ && reprepro -v -v -v update > %s",
		workdir,
		repository.DistCodename,
		logPath,
	)
	log.Println(cmdStr)
	cmd := exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	return
}
