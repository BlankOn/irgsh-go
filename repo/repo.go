package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

func Repo(payload string) (err error) {
	fmt.Println("========== Submitting the package into the repository")
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	logPath := irgshConfig.Repo.Workdir + "/artifacts/" + raw["taskUUID"].(string) + "/repo.log"

	cmdStr := fmt.Sprintf("mkdir -p %s/artifacts/ && cd %s/artifacts/ && wget %s/%s.tar.gz && tar -xvf %s.tar.gz",
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.Workdir,
		irgshConfig.Chief.Address,
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
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		irgshConfig.Repo.DistCodename,
		irgshConfig.Repo.Workdir,
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

	logPath := irgshConfig.Repo.Workdir + "/init.log"
	go StreamLog(logPath)

	cmdStr := fmt.Sprintf("mkdir -p %s && rm -rf %s/%s; cp -vR /usr/share/irgsh/reprepro-template %s/%s",
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
	)
	cmd := exec.Command("bash", "-c", cmdStr)
	_ = cmd.Run()

	cmdStr = fmt.Sprintf(`cd %s/%s/conf && cat updates.orig | 
		sed 's/UPSTREAM_NAME/%s/g' | 
		sed 's/UPSTREAM_DIST_CODENAME/%s/g' | 
		sed 's/UPSTREAM_DIST_URL/%s/g' | 
		sed 's/DIST_SUPPORTED_ARCHITECTURES/%s/g' | 
		sed 's/UPSTREAM_DIST_COMPONENTS/%s/g' > updates && rm updates.orig`,
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		irgshConfig.Repo.UpstreamName,
		irgshConfig.Repo.UpstreamDistCodename,
		strings.Replace(irgshConfig.Repo.UpstreamDistUrl, "/", "\\/", -1),
		irgshConfig.Repo.DistSupportedArchitectures,
		irgshConfig.Repo.UpstreamDistComponents,
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
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
		irgshConfig.Repo.DistName,
		irgshConfig.Repo.DistLabel,
		irgshConfig.Repo.DistCodename,
		irgshConfig.Repo.DistComponents,
		irgshConfig.Repo.DistSupportedArchitectures,
		irgshConfig.Repo.DistVersionDesc,
		irgshConfig.Repo.DistVersion,
		irgshConfig.Repo.DistSigningKey,
		irgshConfig.Repo.UpstreamName,
	)
	fmt.Println(cmdStr)
	cmd = exec.Command("bash", "-c", cmdStr)
	err = cmd.Run()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	repositoryPath := strings.Replace(irgshConfig.Repo.Workdir+"/"+irgshConfig.Repo.DistCodename, "/", "\\/", -1)
	cmdStr = fmt.Sprintf("cd %s/%s/conf && cat options.orig | sed 's/IRGSH_REPO_WORKDIR/%s/g' > options && rm options.orig",
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
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
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
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
	fmt.Println("Syncing irgshConfig.Repo.against %s at %s...", irgshConfig.Repo.UpstreamDistCodename, irgshConfig.Repo.UpstreamDistUrl)

	logPath := irgshConfig.Repo.Workdir + "/update.log"
	go StreamLog(logPath)

	cmdStr := fmt.Sprintf("cd %s/%s/ && reprepro -v -v -v update > %s",
		irgshConfig.Repo.Workdir,
		irgshConfig.Repo.DistCodename,
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
