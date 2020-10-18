package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/blankon/irgsh-go/pkg/systemutil"
	"github.com/google/uuid"
	"github.com/manifoldco/promptui"
)

func InitBase() (err error) {
	logPath := irgshConfig.Builder.Workdir
	logPath += "/irgsh-builder-init-base-" + uuid.New().String() + ".log"
	go systemutil.StreamLog(logPath)

	cmdStr := "lsb_release -a | grep Distributor | cut -d ':' -f 2 | awk '{print $1=$1;1}'"
	distribution, _ := systemutil.CmdExec(
		cmdStr,
		"",
		logPath,
	)

	// TODO base.tgz file name should be based on distribution code name
	fmt.Println("WARNING: This subcommand need to be run under root or sudo.")
	prompt := promptui.Prompt{
		Label:     "irgsh-builder init-base will create (or recreate if already exists) the pbuilder base.tgz on your system and may took long time to be complete. Are you sure?",
		IsConfirm: true,
	}
	result, promptErr := prompt.Run()
	// Avoid shadowed err
	err = promptErr
	if err != nil {
		return
	}
	if strings.ToLower(result) != "y" {
		return
	}

	fmt.Println("Installing and preparing pbuilder and friends...")

	cmdStr = "apt-get update && apt-get install -y pbuilder debootstrap devscripts equivs"
	_, err = systemutil.CmdExec(
		cmdStr,
		"Preparing pbuilder and it's dependencies",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = "rm /var/cache/pbuilder/base*"
	_, _ = systemutil.CmdExec(
		cmdStr,
		"",
		logPath,
	)

	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	cmdStr = "pbuilder create --debootstrapopts --variant=buildd"
	if strings.Contains(irgshConfig.Repo.UpstreamDistUrl, "debian") && strings.Contains(distribution, "Ubuntu") {
		_, err = systemutil.CmdExec(
			"apt-get update && apt-get -y install debian-archive-keyring",
			"Creating pbuilder base.tgz",
			logPath,
		)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			return
		}
		cmdStr = "pbuilder create --distribution " + irgshConfig.Repo.UpstreamDistCodename + " --mirror " + irgshConfig.Repo.UpstreamDistUrl + " --debootstrapopts \"--keyring=/usr/share/keyrings/debian-archive-keyring.gpg\""
	}
	_, err = systemutil.CmdExec(
		cmdStr,
		"Creating pbuilder base.tgz",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = "pbuilder update"
	_, err = systemutil.CmdExec(
		cmdStr,
		"Updating base.tgz",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = "chmod a+rw /var/cache/pbuilder/base*"
	_, err = systemutil.CmdExec(
		cmdStr,
		"Fixing permission for /var/cache/pbuilder/base*",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	if irgshConfig.IsDev {
		cwd, _ := os.Getwd()
		tmpDir := cwd + "/tmp/"
		cmdStr = "chmod -vR 777 " + tmpDir
		_, err = systemutil.CmdExec(
			cmdStr,
			"Fixing permission for "+tmpDir,
			logPath,
		)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			return
		}
	}

	fmt.Println("Done.")

	return
}

func UpdateBase() (err error) {
	fmt.Println("WARNING: This subcommand need to be run under root or sudo.")
	logPath := "/tmp/irgsh-builder-update-base-" + uuid.New().String() + ".log"
	go systemutil.StreamLog(logPath)

	fmt.Println("Updating base.tgz...")
	cmdStr := "sudo pbuilder update"
	_, err = systemutil.CmdExec(
		cmdStr,
		"Updating base.tgz",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Println("Done.")

	return
}

func InitBuilder() (err error) {
	logPath := irgshConfig.Builder.Workdir
	logPath += "/irgsh-builder-init-" + uuid.New().String() + ".log"
	go systemutil.StreamLog(logPath)

	fmt.Println("Preparing containerized pbuilder...")

	cmdStr := `mkdir -p ` + irgshConfig.Builder.Workdir + `/pbocker && \
    cp /var/cache/pbuilder/base.tgz ` + irgshConfig.Builder.Workdir + `/pbocker/base.tgz`
	_, err = systemutil.CmdExec(
		cmdStr,
		"Copying base.tgz",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = `echo 'FROM debian:latest' > ` + irgshConfig.Builder.Workdir + `/pbocker/Dockerfile && \
    echo 'RUN apt-get update && apt-get -y install pbuilder' >> ` + irgshConfig.Builder.Workdir + `/pbocker/Dockerfile && \
    echo 'RUN echo "MIRRORSITE=` + irgshConfig.Builder.UpstreamDistUrl + `" > /root/.pbuilderrc' >> ` + irgshConfig.Builder.Workdir + `/pbocker/Dockerfile && \
    echo 'RUN echo "USENETWORK=yes"' >> ` + irgshConfig.Builder.Workdir + `/pbocker/Dockerfile && \
    echo 'COPY base.tgz /var/cache/pbuilder/' >> ` + irgshConfig.Builder.Workdir + `/pbocker/Dockerfile && \
    echo 'RUN echo "pbuilder --build /tmp/build/*.dsc && cp -vR /var/cache/pbuilder/result/* /tmp/build/" > /build.sh && chmod a+x /build.sh' >> ` + irgshConfig.Builder.Workdir + `/pbocker/Dockerfile`
	_, err = systemutil.CmdExec(
		cmdStr,
		"Preparing Dockerfile",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	cmdStr = `cd ` + irgshConfig.Builder.Workdir +
		`/pbocker && docker build --no-cache -t pbocker .`
	_, err = systemutil.CmdExec(
		cmdStr,
		"Building pbocker docker image",
		logPath,
	)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	fmt.Println("Done.")

	return
}
