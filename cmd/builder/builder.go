package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/blankon/irgsh-go/pkg/systemutil"
	"gopkg.in/src-d/go-git.v4"
)

func uploadLog(logPath string, id string) {
	// Upload the log to chief
	cmdStr := "curl -v -F 'uploadFile=@" + logPath + "' '"
	cmdStr += irgshConfig.Chief.Address + "/api/v1/log-upload?id=" + id + "&type=build'"
	_, err := systemutil.CmdExec(
		cmdStr,
		"Uploading log file to chief",
		"",
	)
	if err != nil {
		fmt.Println(err.Error())
	}
}

// Main task wrapper
func Build(payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	fmt.Println("Processing pipeline :" + raw["taskUUID"].(string))

	logPath := irgshConfig.Builder.Workdir + "/" + raw["taskUUID"].(string) + "/build.log"
	go systemutil.StreamLog(logPath)

	next, err = Clone(payload)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		uploadLog(logPath, raw["taskUUID"].(string))
		return
	}

	next, err = BuildPreparation(payload)
	if err != nil {
		uploadLog(logPath, raw["taskUUID"].(string))
		return
	}

	next, err = BuildPackage(payload)
	if err != nil {
		uploadLog(logPath, raw["taskUUID"].(string))
		return
	}

	next, err = StorePackage(payload)

	if err != nil {
		uploadLog(logPath, raw["taskUUID"].(string))
		return
	}

	uploadLog(logPath, raw["taskUUID"].(string))

	fmt.Println("Done.")

	return
}

func Clone(payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	// Cloning source files
	sourceURL := raw["sourceUrl"].(string)
	_, err = git.PlainClone(
		irgshConfig.Builder.Workdir+"/"+raw["taskUUID"].(string)+"/source",
		false,
		&git.CloneOptions{
			URL:      sourceURL,
			Progress: os.Stdout,
		},
	)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	// Cloning Debian package files
	packageURL := raw["packageUrl"].(string)
	_, err = git.PlainClone(
		irgshConfig.Builder.Workdir+"/"+raw["taskUUID"].(string)+"/package",
		false,
		&git.CloneOptions{
			URL:      packageURL,
			Progress: os.Stdout,
		},
	)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	time.Sleep(0 * time.Second)

	next = payload
	return
}

func BuildPreparation(payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	tarballB64 := raw["tarball"].(string)

	buff, err := base64.StdEncoding.DecodeString(tarballB64)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	err = ioutil.WriteFile(
		irgshConfig.Builder.Workdir+"/"+raw["taskUUID"].(string)+"/debuild.tar.gz",
		buff,
		07440,
	)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	// Extract signed DSC
	cmdStr := "cd " + irgshConfig.Builder.Workdir + "/" + raw["taskUUID"].(string)
	cmdStr += " && tar -xvf debuild.tar.gz && rm -f debuild.tar.gz"
	_, err = systemutil.CmdExec(
		cmdStr,
		"",
		"",
	)
	if err != nil {
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

	buildPath := irgshConfig.Builder.Workdir + "/" + raw["taskUUID"].(string)
	err = os.MkdirAll(buildPath, 0755)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	logPath := buildPath + "/build.log"

	// Copy the source files
	cmdStr := "cp -vR " + buildPath
	cmdStr += "/source/* " + irgshConfig.Builder.Workdir + "/" + raw["taskUUID"].(string)
	cmdStr += "/package/"
	_, err = systemutil.CmdExec(
		cmdStr,
		"",
		logPath,
	)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	// Cleanup pbuilder cache result
	_, _ = systemutil.CmdExec(
		"rm -rf /var/cache/pbuilder/result/*",
		"",
		"",
	)

	// Building the package
	cmdStr = "docker run -v " + irgshConfig.Builder.Workdir + "/" + raw["taskUUID"].(string)
	cmdStr += ":/tmp/build --privileged=true -i pbocker bash -c /build.sh"
	fmt.Println(cmdStr)
	_, err = systemutil.CmdExec(
		cmdStr,
		"Building the package",
		logPath,
	)
	if err != nil {
		log.Println(err.Error())
		return
	}

	next = payload
	return
}

func StorePackage(payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	logPath := irgshConfig.Builder.Workdir + "/" + raw["taskUUID"].(string) + "/build.log"

	cmdStr := "cd " + irgshConfig.Builder.Workdir + " && "
	cmdStr += "tar -zcvf " + raw["taskUUID"].(string) + ".tar.gz " + raw["taskUUID"].(string)
	cmdStr += " && curl -v -F 'uploadFile=@" + irgshConfig.Builder.Workdir
	cmdStr += "/" + raw["taskUUID"].(string) + ".tar.gz' "
	cmdStr += irgshConfig.Chief.Address + "/api/v1/artifact-upload?id="
	cmdStr += raw["taskUUID"].(string)
	_, err = systemutil.CmdExec(
		cmdStr,
		"",
		logPath,
	)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	next = payload
	return
}
