package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"strings"

	"github.com/google/uuid"
	"github.com/imroc/req"
	"github.com/urfave/cli"
	"gopkg.in/src-d/go-git.v4"
)

var (
	app                  *cli.App
	homeDir              string
	chiefAddress         string
	maintainerSigningKey string
	sourceUrl            string
	packageUrl           string
	pipelineId           string
	irgshConfig          IrgshConfig
)

func checkForInitValues() (err error) {
	dat0, _ := ioutil.ReadFile(homeDir + "/.irgsh/IRGSH_CHIEF_ADDRESS")
	chiefAddress = string(dat0)
	dat1, _ := ioutil.ReadFile(homeDir + "/.irgsh/IRGSH_MAINTAINER_SIGNING_KEY")
	maintainerSigningKey = string(dat1)
	if len(chiefAddress) < 1 || len(maintainerSigningKey) < 1 {
		errMsg := "irgsh-cli need to be initialized first. Run: "
		errMsg += "irgsh-cli --chief yourirgshchiefaddress init --key yourgpgkeyfingerprint"
		err = errors.New(errMsg)
		fmt.Println(err.Error())
	}
	return
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	homeDir = usr.HomeDir

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
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:        "chief",
					Value:       "",
					Destination: &chiefAddress,
					Usage:       "Chief address",
				},
				cli.StringFlag{
					Name:        "key",
					Value:       "",
					Destination: &maintainerSigningKey,
					Usage:       "Maintainer signing key",
				},
			},
			Action: func(c *cli.Context) (err error) {
				if len(chiefAddress) < 1 {
					msg := "Chief address should not be empty. Example: "
					msg += "irgsh-cli init --chief https://irgsh.blankonlinux.or.id --key B113D905C417D"
					err = errors.New(msg)
					return
				}
				if len(maintainerSigningKey) < 1 {
					msg := "Signing key should not be empty. Example: "
					msg += "irgsh-cli init --chief https://irgsh.blankonlinux.or.id --key B113D905C417D"
					err = errors.New(msg)
					return
				}
				_, err = url.ParseRequestURI(chiefAddress)
				if err != nil {
					return
				}

				cmdStr := "mkdir -p " + homeDir + "/.irgsh/tmp && echo -n '" + chiefAddress
				cmdStr += "' > " + homeDir + "/.irgsh/IRGSH_CHIEF_ADDRESS"
				cmd := exec.Command("bash", "-c", cmdStr)
				err = cmd.Run()
				if err != nil {
					log.Println(cmdStr)
					log.Println("error: %v\n", err)
					return
				}
				cmdStr = "mkdir -p " + homeDir + "/.irgsh/tmp && echo -n '"
				cmdStr += maintainerSigningKey + "' > " + homeDir + "/.irgsh/IRGSH_MAINTAINER_SIGNING_KEY"
				cmd = exec.Command("bash", "-c", cmdStr)
				err = cmd.Run()
				if err != nil {
					log.Println(cmdStr)
					log.Println("error: %v\n", err)
					return
				}
				fmt.Println("irgsh-cli is successfully initialized. Happy hacking!")
				return err
			},
		},

		{
			Name:  "submit",
			Usage: "Submit new build pipeline",
			Flags: []cli.Flag{
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
			},
			Action: func(c *cli.Context) (err error) {
				err = checkForInitValues()
				if err != nil {
					os.Exit(1)
				}
				if len(sourceUrl) < 1 {
					err = errors.New("--source should not be empty")
					return
				}
				_, err = url.ParseRequestURI(sourceUrl)
				if err != nil {
					return
				}

				if len(packageUrl) < 1 {
					err = errors.New("--package should not be empty")
					return
				}
				_, err = url.ParseRequestURI(packageUrl)
				if err != nil {
					return
				}

				fmt.Println("sourceUrl: " + sourceUrl)
				fmt.Println("packageUrl: " + packageUrl)

				tmpID := uuid.New().String()
				// Cloning Debian package files
				_, err = git.PlainClone(
					"/home/herpiko/.irgsh/tmp/"+tmpID+"/package",
					false,
					&git.CloneOptions{
						URL:      packageUrl,
						Progress: os.Stdout,
					},
				)
				if err != nil {
					fmt.Println(err.Error())
					return
				}

				// Signing DSC
				cmdStr := "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/package && debuild -S -k" + maintainerSigningKey
				fmt.Println(cmdStr)
				err = exec.Command("bash", "-c", cmdStr).Run()
				if err != nil {
					log.Println("error: %v\n", err)
					return
				}

				// Clean up
				cmdStr = "rm -rf " + homeDir + "/.irgsh/tmp/" + tmpID + "/package"
				err = exec.Command("bash", "-c", cmdStr).Run()
				if err != nil {
					return
				}

				// Compressing
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += " && tar -zcvf ../" + tmpID + ".tar.gz ."
				err = exec.Command("bash", "-c", cmdStr).Run()
				if err != nil {
					return err
				}

				// Encoding
				cmdStr = "cd " + homeDir + "/.irgsh/tmp && base64 -w0 " + tmpID + ".tar.gz"
				tarballB64, err := exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					return
				}
				tarballB64Trimmed := strings.TrimSuffix(string(tarballB64), "\n")

				header := make(http.Header)
				header.Set("Accept", "application/json")
				req.SetFlags(req.LrespBody)
				jsonStr := "{\"sourceUrl\":\"" + sourceUrl + "\", \"packageUrl\":\""
				jsonStr += packageUrl + "\", \"tarball\": \"" + tarballB64Trimmed + "\"}"
				result, err := req.Post(chiefAddress+"/api/v1/submit", header, req.BodyJSON(jsonStr))
				if err != nil {
					return
				}

				responseStr := fmt.Sprintf("%+v", result)
				fmt.Println(responseStr)
				if strings.Contains(responseStr, "401 Unauthorized") {
					return
				}
				type SubmitResponse struct {
					PipelineID string `json:"pipelineId"`
				}
				responseJson := SubmitResponse{}
				err = json.Unmarshal([]byte(responseStr), &responseJson)
				if err != nil {
					return
				}
				fmt.Println(responseJson.PipelineID)
				cmdStr = "mkdir -p " + homeDir + "/.irgsh/tmp && echo -n '"
				cmdStr += responseJson.PipelineID + "' > " + homeDir + "/.irgsh/LAST_PIPELINE_ID"
				cmd := exec.Command("bash", "-c", cmdStr)
				err = cmd.Run()
				if err != nil {
					return
				}

				return err
			},
		},
		{
			Name:  "status",
			Usage: "Check status of a pipeline",
			Action: func(c *cli.Context) (err error) {
				pipelineId = c.Args().First()
				err = checkForInitValues()
				if err != nil {
					os.Exit(1)
				}
				if len(pipelineId) < 1 {
					dat, _ := ioutil.ReadFile(homeDir + "/.irgsh/LAST_PIPELINE_ID")
					pipelineId = string(dat)
					if len(pipelineId) < 1 {
						err = errors.New("--pipeline should not be empty")
						return
					}
				}
				fmt.Println("Checking the status of " + pipelineId + " ...")
				req.SetFlags(req.LrespBody)
				result, err := req.Get(chiefAddress+"/api/v1/status?uuid="+pipelineId, nil)
				if err != nil {
					return err
				}

				responseStr := fmt.Sprintf("%+v", result)
				type SubmitResponse struct {
					PipelineID string `json:"pipelineId"`
					State      string `json:"state"`
				}
				responseJson := SubmitResponse{}
				err = json.Unmarshal([]byte(responseStr), &responseJson)
				if err != nil {
					return
				}
				fmt.Println(responseJson.State)

				return
			},
		},
		{
			Name:  "log",
			Usage: "Read the logs of a pipeline",
			Action: func(c *cli.Context) (err error) {
				pipelineId = c.Args().First()
				err = checkForInitValues()
				if err != nil {
					os.Exit(1)
				}
				if len(pipelineId) < 1 {
					dat, _ := ioutil.ReadFile(homeDir + "/.irgsh/LAST_PIPELINE_ID")
					pipelineId = string(dat)
					if len(pipelineId) < 1 {
						err = errors.New("--pipeline should not be empty")
						return
					}
				}
				fmt.Println("Fetching the logs of " + pipelineId + " ...")
				req.SetFlags(req.LrespBody)
				result, err := req.Get(chiefAddress+"/api/v1/status?uuid="+pipelineId, nil)
				if err != nil {
					return err
				}

				responseStr := fmt.Sprintf("%+v", result)
				type SubmitResponse struct {
					PipelineID string `json:"pipelineId"`
					State      string `json:"state"`
				}
				responseJson := SubmitResponse{}
				err = json.Unmarshal([]byte(responseStr), &responseJson)
				if err != nil {
					return
				}
				if responseJson.State == "STARTED" {
					fmt.Println("The pipeline is not finished yet")
					return
				}

				result, err = req.Get(chiefAddress+"/logs/"+pipelineId+".build.log", nil)
				if err != nil {
					return err
				}
				fmt.Println(fmt.Sprintf("%+v", result))

				result, err = req.Get(chiefAddress+"/logs/"+pipelineId+".repo.log", nil)
				if err != nil {
					return err
				}
				fmt.Println(fmt.Sprintf("%+v", result))

				return
			},
		},
	}

	err = app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
