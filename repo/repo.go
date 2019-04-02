package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func Repo(payload string) (err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	logPath := workdir + "/artifacts/" + raw["taskUUID"].(string) + "/repo.log"

	cmdStr := fmt.Sprintf("mkdir -p %s/artifacts/ && cd %s/artifacts/ && wget %s/%s.tar.gz && tar -xvf %s.tar.gz",
		workdir,
		workdir,
		chiefAddress,
		raw["taskUUID"].(string),
		raw["taskUUID"].(string),
	)
	fmt.Println(cmdStr)
	cmd := exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf("cd %s/%s/ && reprepro -v -v -v includedeb %s %s/artifacts/%s/*.deb >>  %s",
		workdir,
		repository.DistCodename,
		repository.DistCodename,
		workdir,
		raw["taskUUID"],
		logPath,
	)
	fmt.Println(cmdStr)
	cmd = exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	return
}

func InitRepo() (err error) {
	fmt.Println("========== Initializing new repository")

	logPath := workdir + "/init.log"
	go StreamLog(logPath)

	cmdStr := fmt.Sprintf("mkdir -p %s && rm -rf %s/%s; cp -vR /usr/share/irgsh/reprepro-template %s/%s",
		workdir,
		workdir,
		repository.DistCodename,
		workdir,
		repository.DistCodename,
	)
	cmd := exec.Command("bash", "-c", cmdStr)
	_ = cmd.Run()

	cmdStr = fmt.Sprintf(`cd %s/%s/conf && cat updates.orig | 
		sed 's/UPSTREAM_NAME/%s/g' | 
		sed 's/UPSTREAM_DIST_CODENAME/%s/g' | 
		sed 's/UPSTREAM_DIST_URL/%s/g' | 
		sed 's/DIST_SUPPORTED_ARCHITECTURES/%s/g' | 
		sed 's/UPSTREAM_DIST_COMPONENTS/%s/g' > updates && rm updates.orig`,
		workdir,
		repository.DistCodename,
		repository.UpstreamName,
		repository.UpstreamDistCodename,
		strings.Replace(repository.UpstreamDistUrl, "/", "\\/", -1),
		repository.DistSupportedArchitectures,
		repository.UpstreamDistComponents,
	)
	fmt.Println(cmdStr)
	cmd = exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf(`cd %s/%s/conf && cat distributions.orig |
		sed 's/DIST_NAME/%s/g' |
		sed 's/DIST_LABEL/%s/g' |
		sed 's/DIST_CODENAME/%s/g' |
		sed 's/DIST_COMPONENTS/%s/g' |
		sed 's/DIST_SUPPORTED_ARCHITECTURES/%s/g' |
		sed 's/DIST_VERSION_DESC/%s/g' |
		sed 's/DIST_VERSION/%s/g' |
		sed 's/DIST_SIGNING_KEY/%s/g' |
		sed 's/UPSTREAM_NAME/%s/g'> distributions && rm distributions.orig`,
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
	fmt.Println(cmdStr)
	cmd = exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	repositoryPath := strings.Replace(workdir+"/"+repository.DistCodename, "/", "\\/", -1)
	cmdStr = fmt.Sprintf("cd %s/%s/conf && cat options.orig | sed 's/IRGSH_REPO_WORKDIR/%s/g' > options && rm options.orig",
		workdir,
		repository.DistCodename,
		repositoryPath,
	)
	fmt.Println(cmdStr)
	cmd = exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = fmt.Sprintf("cd %s/%s/ && reprepro -v -v -v export > %s",
		workdir,
		repository.DistCodename,
		logPath,
	)
	fmt.Println(cmdStr)
	cmd = exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		fmt.Printf("error: %v\n", err)
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
	fmt.Println(cmdStr)
	cmd := exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	return
}
