package main

import (
	"fmt"
	"os/exec"

	"github.com/google/uuid"
)

func InitBase() (err error) {
	logPath := irgshConfig.Builder.Workdir + "/irgsh-builder-init-" + uuid.New().String() + ".log"
	go StreamLog(logPath)

	fmt.Println("Installing pbuilder and friends...")

	cmdStr := "apt-get update && apt-get install -y pbuilder debootstrap devscripts equivs > " + logPath
	fmt.Println(cmdStr)
	err = exec.Command("bash", "-c", cmdStr).Run()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return err
	}

	_ = exec.Command("bash", "-c", "rm /var/cache/pbuilder/base*").Run()

	cmdStr = "pbuilder create --debootstrapopts --variant=buildd >> " + logPath
	fmt.Println(cmdStr)
	err = exec.Command("bash", "-c", cmdStr).Run()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return err
	}

	fmt.Println(cmdStr)
	err = exec.Command("bash", "-c", "pbuilder update >> "+logPath).Run()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return err
	}

	return nil
}

func UpdateBase() (err error) {
	logPath := "/tmp/irgsh-builder-init-" + uuid.New().String() + ".log"
	go StreamLog(logPath)

	fmt.Println("Updating base.tgz...")
	cmdStr := "sudo pbuilder update >> " + logPath
	fmt.Println(cmdStr)
	err = exec.Command("bash", "-c", cmdStr).Run()
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return err
	}

	return nil
}
