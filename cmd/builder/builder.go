package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/blankon/irgsh-go/pkg/systemutil"
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

	logPath := irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string) + "/build.log"
	go systemutil.StreamLog(logPath)

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

func BuildPreparation(payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	buildPath := irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
	logPath := buildPath + "/build.log"

	targetDir := irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
	err = os.MkdirAll(targetDir, 0755)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	target := irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string) + "/debuild.tar.gz"
	// Downloading the submission tarball from chief
	cmdStr := "curl -v -o " + target + " "
	cmdStr += irgshConfig.Chief.Address + "/submissions/" + raw["taskUUID"].(string) + ".tar.gz"
	log.Println(cmdStr)
	_, err = systemutil.CmdExec(
		cmdStr,
		"Fetching the submission tarball from chief",
		logPath,
	)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	// Extract the signed dsc
	cmdStr = "cd " + irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
	cmdStr += " && tar -xvf debuild.tar.gz "
	cmdStr += " && rm -rf debuild.tar.gz "
	_, err = systemutil.CmdExec(
		cmdStr,
		"Backup the maintainer tarball and its signature",
		logPath,
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

	buildPath := irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
	err = os.MkdirAll(buildPath, 0755)
	if err != nil {
		log.Printf("error: %v\n", err)
		return
	}

	logPath := buildPath + "/build.log"

	packageNameVersion := raw["packageName"].(string) + "-" + raw["packageVersion"].(string)
	if len(raw["packageExtendedVersion"].(string)) > 0 {
		packageNameVersion += "-" + raw["packageExtendedVersion"].(string)
	}

	// Copy the maintainer's generated files from signed dir
	if len(raw["sourceUrl"].(string)) > 0 {
		cmdStr := "cd " + irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
		cmdStr += " && cp signed/* ."
		log.Println(cmdStr)
		_, err = systemutil.CmdExec(
			cmdStr,
			"Copy the maintainer's generated files from signed dir.",
			logPath,
		)
		if err != nil {
			log.Printf("error: %v\n", err)
			return
		}
	}

	// Cleanup pbuilder cache result
	_, _ = systemutil.CmdExec(
		"rm -rf /var/cache/pbuilder/result/*",
		"",
		"",
	)

	// Building the package
	cmdStr := "docker run -v " + irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
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

	// Use the generated files from maintainer
	if len(raw["sourceUrl"].(string)) > 0 {
		cmdStr := "cd " + irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string)
		cmdStr += " && cp signed/* . "
		log.Println(cmdStr)
		_, err = systemutil.CmdExec(
			cmdStr,
			"Use the generated files from maintainer.",
			logPath,
		)
		if err != nil {
			log.Printf("error: %v\n", err)
			return
		}
	}

	next = payload
	return
}

func StorePackage(payload string) (next string, err error) {
	in := []byte(payload)
	var raw map[string]interface{}
	json.Unmarshal(in, &raw)

	logPath := irgshConfig.Builder.Workdir + "/artifacts/" + raw["taskUUID"].(string) + "/build.log"

	cmdStr := "cd " + irgshConfig.Builder.Workdir + "/artifacts/ && "
	cmdStr += "tar -zcvf " + raw["taskUUID"].(string) + ".tar.gz " + raw["taskUUID"].(string)
	cmdStr += " && curl -v -F 'uploadFile=@" + irgshConfig.Builder.Workdir
	cmdStr += "/artifacts/" + raw["taskUUID"].(string) + ".tar.gz' "
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
