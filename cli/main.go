package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"

	"github.com/imroc/req"
	"github.com/urfave/cli"
)

var (
	app          *cli.App
	chiefAddress string
	sourceUrl    string
	packageUrl   string
	pipelineId   string
)

func checkForChief() (err error) {
	usr, err := user.Current()
	if err != nil {
		return
	}
	dat, _ := ioutil.ReadFile(usr.HomeDir + "/.irgsh/IRGSH_CHIEF_ADDRESS")
	chiefAddress = string(dat)
	if len(chiefAddress) < 1 {
		err = errors.New("irgsh-cli need to be initialized first. Run: irgsh-cli --chief yourirgshchiefaddress init")
		fmt.Println(err.Error())
	}
	return
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	app = cli.NewApp()
	app.Name = "irgsh-go"
	app.Usage = "irgsh-go distributed packager"
	app.Author = "BlankOn Developer"
	app.Email = "blankon-dev@googlegroups.com"
	app.Version = "IRGSH_GO_VERSION"

	app.Commands = []cli.Command{

		{
			Name:  "init",
			Usage: "Initialize irgsh-cli",
			Action: func(c *cli.Context) (err error) {
				if len(chiefAddress) < 1 {
					err = errors.New("--chief should not be empty")
					return
				}
				cmdStr := "mkdir -p ~/.irgsh && echo -n '" + chiefAddress + "' > ~/.irgsh/IRGSH_CHIEF_ADDRESS"
				cmd := exec.Command("bash", "-c", cmdStr)
				err = cmd.Run()
				if err != nil {
					log.Println(cmdStr)
					log.Printf("error: %v\n", err)
					return
				}
				fmt.Println("irgsh-cli is successfully initialized. Happy hacking!")
				return err
			},
		},

		{
			Name:  "submit",
			Usage: "Submit new build pipeline",
			Action: func(c *cli.Context) (err error) {
				err = checkForChief()
				if err != nil {
					os.Exit(1)
				}
				if len(sourceUrl) < 1 {
					err = errors.New("--source should not be empty")
					return
				}
				if len(packageUrl) < 1 {
					err = errors.New("--package should not be empty")
					return
				}
				fmt.Println("sourceUrl: " + sourceUrl)
				fmt.Println("packageUrl: " + packageUrl)

				header := make(http.Header)
				header.Set("Accept", "application/json")
				req.SetFlags(req.LrespBody)
				result, err := req.Post(chiefAddress+"/api/v1/submit", header, req.BodyJSON("{\"sourceUrl\":\""+sourceUrl+"\", \"packageUrl\":\""+packageUrl+"\"}"))
				if err != nil {
					fmt.Println(err.Error())
				}
				fmt.Printf("%+v", result)
				return err
			},
		},
		{
			Name:  "status",
			Usage: "Check status of a pipeline",
			Action: func(c *cli.Context) (err error) {
				err = checkForChief()
				if err != nil {
					os.Exit(1)
				}
				if len(pipelineId) < 1 {
					err = errors.New("--pipeline should not be empty")
					return
				}
				fmt.Println("Checking the status of " + pipelineId + "...")
				req.SetFlags(req.LrespBody)
				result, _ := req.Get(chiefAddress+"/api/v1/status?uuid="+pipelineId, nil)
				fmt.Printf("%+v", result)
				return err
			},
		},
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "chief",
			Value:       "",
			Destination: &chiefAddress,
			Usage:       "irgsh-chief address",
		},
		cli.StringFlag{
			Name:        "source",
			Value:       "",
			Destination: &sourceUrl,
			Usage:       "Source URL",
		},
		cli.StringFlag{
			Name:        "package",
			Value:       "",
			Destination: &packageUrl,
			Usage:       "Package URL",
		},
		cli.StringFlag{
			Name:        "pipeline",
			Value:       "",
			Destination: &pipelineId,
			Usage:       "Pipeline ID",
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
