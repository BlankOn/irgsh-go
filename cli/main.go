package main

import (
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
	dat, _ := ioutil.ReadFile(homeDir + "/.irgsh/IRGSH_CHIEF_ADDRESS")
	chiefAddress = string(dat)
	if len(chiefAddress) < 1 {
		err = errors.New("irgsh-cli need to be initialized first. Run: irgsh-cli --chief yourirgshchiefaddress init --key yourgpgkeyfingerprint")
		fmt.Println(err.Error())
	}
	dat, _ = ioutil.ReadFile(homeDir + "/.irgsh/IRGSH_MAINTAINER_SIGNING_KEY")
	maintainerSigningKey = string(dat)
	if len(maintainerSigningKey) < 1 {
		err = errors.New("irgsh-cli need to be initialized first. Run: irgsh-cli --chief yourirgshchiefaddress init --key yourgpgkeyfingerprint")
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
					err = errors.New("Chief address should not be empty. Example: irgsh-cli init --chief https://irgsh.blankonlinux.or.id --key B113D905C417D9C31DAD9F0E509A356412B6E77F")
					return
				}
				if len(maintainerSigningKey) < 1 {
					err = errors.New("Signing key should not be empty. Example: irgsh-cli init --chief https://irgsh.blankonlinux.or.id --key B113D905C417D9C31DAD9F0E509A356412B6E77F")
					return
				}
				_, err = url.ParseRequestURI(chiefAddress)
				if err != nil {
					return
				}

				cmdStr := "mkdir -p " + homeDir + "/.irgsh/tmp && echo -n '" + chiefAddress + "' > " + homeDir + "/.irgsh/IRGSH_CHIEF_ADDRESS"
				cmd := exec.Command("bash", "-c", cmdStr)
				err = cmd.Run()
				if err != nil {
					log.Println(cmdStr)
					log.Printf("error: %v\n", err)
					return
				}
				cmdStr = "mkdir -p " + homeDir + "/.irgsh/tmp && echo -n '" + maintainerSigningKey + "' > " + homeDir + "/.irgsh/IRGSH_MAINTAINER_SIGNING_KEY"
				cmd = exec.Command("bash", "-c", cmdStr)
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
				_, err = git.PlainClone("/home/herpiko/.irgsh/tmp/"+tmpID+"/package", false, &git.CloneOptions{
					URL:      packageUrl,
					Progress: os.Stdout,
				})
				if err != nil {
					fmt.Println(err.Error())
					return
				}

				// Signing DSC
				cmdStr := "cd " + homeDir + "/.irgsh/tmp/" + tmpID + "/package && debuild -S -k" + maintainerSigningKey
				fmt.Println(cmdStr)
				err = exec.Command("bash", "-c", cmdStr).Run()
				if err != nil {
					log.Printf("error: %v\n", err)
					return
				}

				// Clean up
				cmdStr = "rm -rf " + homeDir + "/.irgsh/tmp/" + tmpID + "/package"
				fmt.Println(cmdStr)
				err = exec.Command("bash", "-c", cmdStr).Run()
				if err != nil {
					log.Printf("error: %v\n", err)
					return
				}

				// Compressing
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID + " && tar -zcvf ../" + tmpID + ".tar.gz ."
				fmt.Println(cmdStr)
				err = exec.Command("bash", "-c", cmdStr).Run()
				if err != nil {
					log.Printf("error: %v\n", err)
					return
				}

				// Encoding
				cmdStr = "cd " + homeDir + "/.irgsh/tmp && base64 -w0 " + tmpID + ".tar.gz"
				fmt.Println(cmdStr)
				tarballB64, err := exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					log.Printf("error: %v\n", err)
					return
				}
				tarballB64Trimmed := strings.TrimSuffix(string(tarballB64), "\n")

				header := make(http.Header)
				header.Set("Accept", "application/json")
				req.SetFlags(req.LrespBody)
				jsonStr := "{\"sourceUrl\":\"" + sourceUrl + "\", \"packageUrl\":\"" + packageUrl + "\", \"tarball\": \"" + tarballB64Trimmed + "\"}"
				fmt.Println(jsonStr)
				result, err := req.Post(chiefAddress+"/api/v1/submit", header, req.BodyJSON(jsonStr))
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
				pipelineId = c.Args().First()
				err = checkForInitValues()
				if err != nil {
					os.Exit(1)
				}
				if len(pipelineId) < 1 {
					err = errors.New("--pipeline should not be empty")
					return
				}
				fmt.Println("Checking the status of " + pipelineId + "...")
				req.SetFlags(req.LrespBody)
				result, err := req.Get(chiefAddress+"/api/v1/status?uuid="+pipelineId, nil)
				if err != nil {
					log.Println(err.Error())
				}
				fmt.Printf("%+v", result)
				return err
			},
		},
	}

	err = app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
