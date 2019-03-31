package main

import (
	"log"
	"os/exec"

	"github.com/google/uuid"
)

func Execute(command string) (err error) {
	cmd := exec.Command("bash", "-c", command)
	err = cmd.Run()

	/*
	   var stdoutBuff bytes.Buffer
	   var stderrBuff bytes.Buffer
	   cmd.Stdout = &stdoutBuff
	   cmd.Stderr = &stderrBuff
	*/

	return err
}

func InitBase() (err error) {
	logPath := "/tmp/irgsh-builder-init-" + uuid.New().String() + ".log"
	go StreamLog(logPath)

	log.Println("Installing pbuilder and friends...")

	err = Execute("sudo apt-get install pbuilder debootstrap devscripts equivs > " + logPath)
	if err != nil {
		log.Printf("error: %v\n", err)
		return err
	}

	_ = Execute("sudo rm /var/cache/pbuilder/base.*")

	err = Execute("sudo pbuilder create --debootstrapopts --variant=buildd >> " + logPath)
	if err != nil {
		log.Printf("error: %v\n", err)
		return err
	}

	err = Execute("sudo pbuilder update >> " + logPath)
	if err != nil {
		log.Printf("error: %v\n", err)
		return err
	}

	return nil
}

func UpdateBase() (err error) {
	logPath := "/tmp/irgsh-builder-init-" + uuid.New().String() + ".log"
	go StreamLog(logPath)

	log.Println("Updating base.tgz...")

	err = Execute("sudo pbuilder update >> " + logPath)
	if err != nil {
		log.Printf("error: %v\n", err)
		return err
	}

	return nil
}
