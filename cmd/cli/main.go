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
	"github.com/manifoldco/promptui"
	"github.com/urfave/cli"
	"gopkg.in/src-d/go-git.v4"
)

type Submission struct {
	PackageName           string `json:"packageName"`
	PackageURL            string `json:"packageUrl"`
	SourceURL             string `json:"sourceUrl"`
	Maintainer            string `json:"maintainer"`
	MaintainerFingerprint string `json:"maintainerFingerprint"`
	Component             string `json:"component"`
	IsExperimental        bool   `json:"isExperimental"`
	Tarball               string `json:"tarball"`
}

var (
	app                  *cli.App
	homeDir              string
	chiefAddress         string
	maintainerSigningKey string
	sourceUrl            string
	component            string
	packageUrl           string
	version              string
	isExperimental       bool
	pipelineId           string
)

func checkForInitValues() (err error) {
	dat0, _ := ioutil.ReadFile(homeDir + "/.irgsh/IRGSH_CHIEF_ADDRESS")
	chiefAddress = string(dat0)
	dat1, _ := ioutil.ReadFile(homeDir + "/.irgsh/IRGSH_MAINTAINER_SIGNING_KEY")
	maintainerSigningKey = string(dat1)
	if len(chiefAddress) < 1 || len(maintainerSigningKey) < 1 {
		errMsg := "irgsh-cli need to be configured first. Run: "
		errMsg += "irgsh-cli config --chief yourirgshchiefaddress --key yourgpgkeyfingerprint"
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
	app.Version = version

	app.Commands = []cli.Command{

		{
			Name:  "config",
			Usage: "Configure irgsh-cli",
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
					msg += "irgsh-cli config --chief https://irgsh.blankonlinux.or.id --key B113D905C417D"
					err = errors.New(msg)
					return
				}
				if len(maintainerSigningKey) < 1 {
					msg := "Signing key should not be empty. Example: "
					msg += "irgsh-cli config --chief https://irgsh.blankonlinux.or.id --key B113D905C417D"
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
				// TODO test a connection against the chief
				fmt.Println("irgsh-cli is successfully configured. Happy hacking!")
				return err
			},
		},

		{
			Name:  "submit",
			Usage: "Submit new build",
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
				cli.StringFlag{
					Name:        "component",
					Value:       "",
					Destination: &component,
					Usage:       "Repository component",
				},
				cli.BoolFlag{
					Name:  "experimental",
					Usage: "Enable experimental flag",
				},
			},
			Action: func(ctx *cli.Context) (err error) {
				err = checkForInitValues()
				if err != nil {
					os.Exit(1)
				}

				// Check version first
				header := make(http.Header)
				header.Set("Accept", "application/json")
				req.SetFlags(req.LrespBody)

				type VersionResponse struct {
					Version string `json:"version"`
				}
				result, err := req.Get(chiefAddress+"/api/v1/version", nil)
				if err != nil {
					return err
				}
				responseStr := fmt.Sprintf("%+v", result)
				versionResponse := VersionResponse{}
				err = json.Unmarshal([]byte(responseStr), &versionResponse)
				if err != nil {
					return
				}

				if versionResponse.Version != app.Version {
					err = errors.New("Client version mismatch. Please update your irgsh-cli.")
					return
				}

				// Default component is main
				if len(component) < 1 {
					component = "main"
				}

				if len(sourceUrl) > 0 {
					_, err = url.ParseRequestURI(sourceUrl)
					if err != nil {
						return
					}
				}

				if len(packageUrl) < 1 {
					err = errors.New("--package should not be empty")
					return
				}
				_, err = url.ParseRequestURI(packageUrl)
				if err != nil {
					return
				}
				isExperimental = true
				if !ctx.Bool("experimental") {
					prompt := promptui.Prompt{
						Label:     "Experimental flag is not set which means the package will be injected to official dev repository. Are you sure you want to continue to submit and build this package?",
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
					isExperimental = false
				}
				if len(sourceUrl) > 0 {
					fmt.Println("sourceUrl: " + sourceUrl)
				}
				fmt.Println("packageUrl: " + packageUrl)

				tmpID := uuid.New().String()
				// Cloning Debian package files
				_, err = git.PlainClone(
					homeDir+"/.irgsh/tmp/"+tmpID+"/package",
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

				// Getting package name
				packageName := ""
				cmdStr := "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/package && cat debian/control | grep 'Source:' | head -n 1 | cut -d ' ' -f 2"
				fmt.Println(cmdStr)
				output, err := exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					log.Println("error: %v\n", err)
					log.Println("Failed to get package name.")
					return
				}
				packageName = strings.TrimSuffix(string(output), "\n")

				// Getting maintainer identity
				maintainerIdentity := ""
				cmdStr = "gpg -K | grep -A 1 " + maintainerSigningKey + " | tail -n 1 | cut -d ']' -f 2"
				fmt.Println(cmdStr)
				output, err = exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					log.Println("error: %v\n", err)
					log.Println("Failed to get maintainer identity.")
					return
				}
				maintainerIdentity = strings.TrimSuffix(string(output), "\n")

				// Signing DSC
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/package && debuild -S -k" + maintainerSigningKey
				fmt.Println(cmdStr)
				cmd := exec.Command("bash", "-c", cmdStr)
				// Make it interactive
				cmd.Stdout = os.Stdout
				cmd.Stdin = os.Stdin
				cmd.Stderr = os.Stderr
				cmd.Run()
				if err != nil {
					log.Println("error: %v\n", err)
					log.Println("Failed to sign the package using " + maintainerSigningKey + ". Please check your GPG key list.")
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

				submission := Submission{
					PackageName:           packageName,
					PackageURL:            packageUrl,
					SourceURL:             sourceUrl,
					Maintainer:            maintainerIdentity,
					MaintainerFingerprint: maintainerSigningKey,
					Component:             component,
					IsExperimental:        isExperimental,
				}
				jsonByte, _ := json.Marshal(submission)

				// Signing a token
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/ && echo '" + string(jsonByte) + "' > token && gpg --clearsign --output token.sig --sign token"
				fmt.Println(cmdStr)
				cmd = exec.Command("bash", "-c", cmdStr)
				// Make it interactive
				cmd.Stdout = os.Stdout
				cmd.Stdin = os.Stdin
				cmd.Stderr = os.Stderr
				cmd.Run()
				if err != nil {
					log.Println("error: %v\n", err)
					log.Println("Failed to sign the auth token using " + maintainerSigningKey + ". Please check your GPG key list.")
					return
				}

				// Upload
				cmdStr = "curl -v -F 'blob=@" + homeDir + "/.irgsh/tmp/" + tmpID + ".tar.gz" + "' "
				cmdStr += " -F 'token=@" + homeDir + "/.irgsh/tmp/" + tmpID + "/token.sig" + "'"
				cmdStr += " '" + chiefAddress + "/api/v1/submission-upload'"
				output, err = exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					log.Println(output)
					return err
				}
				blobStr := strings.TrimSuffix(string(output), "\n")
				type Blob struct {
					ID string `json:"id"`
				}
				blob := Blob{}
				err = json.Unmarshal([]byte(blobStr), &blob)
				if err != nil {
					return err
				}

				submission.Tarball = blob.ID
				jsonByte, _ = json.Marshal(submission)

				result, err = req.Post(chiefAddress+"/api/v1/submit", header, req.BodyJSON(string(jsonByte)))
				if err != nil {
					return
				}

				responseStr = fmt.Sprintf("%+v", result)
				if !strings.Contains(responseStr, "pipelineId") {
					log.Println(responseStr)
					fmt.Println("Submission failed.")
					return
				}
				type SubmitResponse struct {
					PipelineID string `json:"pipelineId"`
				}
				submissionResponse := SubmitResponse{}
				err = json.Unmarshal([]byte(responseStr), &submissionResponse)
				if err != nil {
					return
				}
				fmt.Println("Submission succeeded. Pipeline ID:")
				fmt.Println(submissionResponse.PipelineID)
				cmdStr = "mkdir -p " + homeDir + "/.irgsh/tmp && echo -n '"
				cmdStr += submissionResponse.PipelineID + "' > " + homeDir + "/.irgsh/LAST_PIPELINE_ID"
				cmd = exec.Command("bash", "-c", cmdStr)
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
