package main

import (
	"bufio"
	"bytes"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/imroc/req"
	"github.com/inconshreveable/go-update"
	"github.com/manifoldco/promptui"
	"github.com/urfave/cli"
)

type Submission struct {
	PackageName            string `json:"packageName"`
	PackageVersion         string `json:"packageVersion"`
	PackageExtendedVersion string `json:"packageExtendedVersion"`
	PackageURL             string `json:"packageUrl"`
	SourceURL              string `json:"sourceUrl"`
	Maintainer             string `json:"maintainer"`
	MaintainerFingerprint  string `json:"maintainerFingerprint"`
	Component              string `json:"component"`
	IsExperimental         bool   `json:"isExperimental"`
	Tarball                string `json:"tarball"`
	PackageBranch          string `json:"packageBranch"`
	SourceBranch           string `json:"sourceBranch"`
}

type GithubReleaseResponse struct {
	Url    string `json:"url"`
	Assets []struct {
		Name               string `json:"name"`
		BrowserDownloadUrl string `json:"browser_download_url"`
	}
}

var (
	app                  *cli.App
	homeDir              string
	chiefAddress         string
	maintainerSigningKey string
	sourceUrl            string
	component            string
	packageBranch        string
	sourceBranch         string
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
					log.Printf("error: %v", err)
					return
				}
				cmdStr = "mkdir -p " + homeDir + "/.irgsh/tmp && echo -n '"
				cmdStr += maintainerSigningKey + "' > " + homeDir + "/.irgsh/IRGSH_MAINTAINER_SIGNING_KEY"
				cmd = exec.Command("bash", "-c", cmdStr)
				err = cmd.Run()
				if err != nil {
					log.Println(cmdStr)
					log.Printf("error: %v", err)
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
				cli.StringFlag{
					Name:        "package-branch",
					Value:       "",
					Destination: &packageBranch,
					Usage:       "package git branch",
				},
				cli.StringFlag{
					Name:        "source-branch",
					Value:       "",
					Destination: &sourceBranch,
					Usage:       "source git branch",
				},
				cli.BoolFlag{
					Name:  "experimental",
					Usage: "Enable experimental flag",
				},
				cli.BoolFlag{
					Name:  "ignore-checks",
					Usage: "Ignoring value checks",
				},
			},
			Action: func(ctx *cli.Context) (err error) {

				ignoreChecks := ctx.Bool("ignore-checks") && ctx.Bool("experimental")

				err = checkForInitValues()
				if err != nil {
					log.Println(err)
					return err
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
					log.Println(err)
					return err
				}
				responseStr := fmt.Sprintf("%+v", result)
				versionResponse := VersionResponse{}
				err = json.Unmarshal([]byte(responseStr), &versionResponse)
				if err != nil {
					log.Println(err)
					return
				}

				if versionResponse.Version != app.Version {
					log.Println("Target version", versionResponse.Version)
					log.Println("Local version", app.Version)
					err = errors.New("Client version mismatch. Please update your irgsh-cli.")
					return
				}

				// Default component is main
				if len(component) < 1 {
					component = "main"
				}

				// Default branch is master
				if len(packageBranch) < 1 {
					packageBranch = "master"
				}
				if len(sourceBranch) < 1 {
					sourceBranch = "master"
				}

				if len(sourceUrl) > 0 {
					_, err = url.ParseRequestURI(sourceUrl)
					if err != nil {
						log.Println(err)
						return
					}
				}

				if len(packageUrl) < 1 {
					err = errors.New("--package should not be empty")
					return
				}
				_, err = url.ParseRequestURI(packageUrl)
				if err != nil {
					log.Println(err)
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
						log.Println(err)
						return
					}
					if strings.ToLower(result) != "y" {
						return
					}
					isExperimental = false
				}
				tmpID := uuid.New().String()
				var downloadableTarballURL string
				if len(sourceUrl) > 0 {
					// TODO Ensure that the debian spec's source format is quilt.
					// Otherwise (native), terminate the submission.
					fmt.Println("sourceUrl: " + sourceUrl)
					// Cloning Debian package files
					err = syncRepo(sourceUrl, sourceBranch, homeDir, homeDir+"/.irgsh/tmp/"+tmpID+"/source")
					if err != nil {
						fmt.Println(err.Error())
						if errors.Is(err, errRepoOrBranchNotFound) {
							// Downloadable tarball? Let's try.
							downloadableTarballURL = strings.TrimSuffix(string(sourceUrl), "\n")
							log.Println(downloadableTarballURL)
							log.Println("Downloading the tarball " + downloadableTarballURL)
							resp, err1 := http.Get(downloadableTarballURL)
							if err1 != nil {
								log.Println(err)
								err = err1
								return
							}
							defer resp.Body.Close()
							// Prepare dirs
							targetDir := homeDir + "/.irgsh/tmp/" + tmpID
							err = os.MkdirAll(targetDir, 0755)
							if err != nil {
								log.Printf("error: %v\n", err)
								return
							}

							// Write the tarball
							out, err := os.Create(targetDir + "/" + path.Base(downloadableTarballURL))
							defer out.Close()
							if err != nil {
								log.Println(err.Error())
								panic(err)
							}
							io.Copy(out, resp.Body)

						} else {
							log.Println(err.Error())
							return
						}
					}
				}
				fmt.Println("packageUrl: " + packageUrl)

				// Cloning Debian package files
				err = syncRepo(packageUrl, packageBranch, homeDir, homeDir+"/.irgsh/tmp/"+tmpID+"/package")
				if err != nil {
					fmt.Println(err.Error())
					return
				}

				var packageName, packageVersion, packageExtendedVersion, packageLastMaintainer, uploaders string

				// Getting package name
				log.Println("Getting package name...")
				cmdStr := "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/package && cat debian/control | grep 'Source:' | head -n 1 | cut -d ' ' -f 2"
				fmt.Println(cmdStr)
				output, err := exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					log.Printf("error: %v", err)
					log.Println("Failed to get package name.")
					return
				}
				packageName = strings.TrimSuffix(string(output), "\n")
				if len(packageName) < 1 {
					log.Println("It seems the repository does not contain debian spec directory.")
					return

				}
				log.Println("Package name: " + packageName)

				// Getting package version
				log.Println("Getting package version ...")
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/package && cat debian/changelog | head -n 1 | cut -d '(' -f 2 | cut -d ')' -f 1 | cut -d '-' -f 1"
				fmt.Println(cmdStr)
				output, err = exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					log.Printf("error: %v", err)
					log.Println("Failed to get package version.")
					return
				}
				packageVersion = strings.TrimSuffix(string(output), "\n")
				if strings.Contains(packageVersion, ":") {
					packageVersion = strings.Split(packageVersion, ":")[1]
				}
				log.Println("Package version: " + packageVersion)

				// Getting package extended version
				log.Println("Getting package extended version ...")
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/package && cat debian/changelog | head -n 1 | cut -d '(' -f 2 | cut -d ')' -f 1 | cut -d '-' -f 2"
				fmt.Println(cmdStr)
				output, err = exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					log.Printf("error: %v", err)
					log.Println("Failed to get package extended version.")
					return
				}
				packageExtendedVersion = strings.TrimSuffix(string(output), "\n")
				if packageExtendedVersion == packageVersion {
					packageExtendedVersion = ""
				}
				log.Println("Package extended version: " + packageExtendedVersion)

				// Getting package last maintainer
				log.Println("Getting package last maintainer ...")
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/package && echo $(cat debian/changelog | grep ' --' | cut -d '-' -f 3 | cut -d '>' -f 1 | head -n 1)'>'"
				fmt.Println(cmdStr)
				output, err = exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					log.Printf("error: %v", err)
					log.Println("Failed to get package extended version.")
					return
				}
				packageLastMaintainer = strings.TrimSuffix(string(output), "\n")
				log.Println(packageLastMaintainer)

				// Getting uploaders
				log.Println("Getting package name...")
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/package && cat debian/control | grep 'Uploaders:' | head -n 1 | cut -d ':' -f 2"
				fmt.Println(cmdStr)
				output, err = exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					log.Printf("error: %v", err)
					log.Println("Failed to get uploaders value.")
					return
				}
				uploaders = strings.TrimSpace(strings.TrimSuffix(string(output), "\n"))

				// Getting maintainer identity
				log.Println("Getting maintainer identity...")
				maintainerIdentity := ""
				cmdStr = "gpg -K | grep -A 1 " + maintainerSigningKey + " | tail -n 1 | cut -d ']' -f 2"
				fmt.Println(cmdStr)
				output, err = exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					log.Printf("error: %v", err)
					log.Println("Failed to get maintainer identity.")
					return
				}
				maintainerIdentity = strings.TrimSpace(strings.TrimSuffix(string(output), "\n"))

				if strings.TrimSpace(uploaders) != strings.TrimSpace(maintainerIdentity) && !ignoreChecks {
					err = errors.New("The uploaders value in the debian/control does not matched with your identity. Please update the debian/control file.")
					log.Println("The uploader in the debian/control: " + uploaders)
					log.Println("Your signing key identity: " + maintainerIdentity)
					return
				}

				if strings.TrimSpace(packageLastMaintainer) != strings.TrimSpace(maintainerIdentity) && !ignoreChecks {
					err = errors.New("The last maintainer in the debian/changelog does not matched with your identity. Please update the debian/changelog file.")
					log.Println("The last maintainer in the debian/changelog: " + packageLastMaintainer)
					log.Println("Your signing key identity: " + maintainerIdentity)
					return
				}

				// Determine package name with version
				log.Println(packageVersion)
				packageNameVersion := packageName + "-" + packageVersion
				log.Println(packageNameVersion)
				log.Println(packageExtendedVersion)
				if len(packageExtendedVersion) > 0 {
					packageNameVersion += "-" + packageExtendedVersion
				}

				if len(sourceUrl) > 0 && len(downloadableTarballURL) < 1 {
					origFileName := packageName + "_" + strings.Split(packageVersion, "-")[0] // Discard quilt revision
					// Compress source to orig tarball
					log.Println("Creating orig tarball...")
					cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
					cmdStr += "/ && mkdir -p tmp && mv source tmp && cd tmp && mv source " + packageName + "-" + packageVersion + " && tar cfJ " + origFileName + ".orig.tar.xz " + packageName + "-" + packageVersion + " && rm -rf " + packageName + "-" + packageVersion + " && mv *.xz .. && cd .. && rm -rf tmp "
					fmt.Println(cmdStr)
					output, err = exec.Command("bash", "-c", cmdStr).Output()
					if err != nil {
						log.Printf("error: %v", err)
						log.Println("Failed to rename workdir.")
					}
				}

				// Rename the package dir so we can run debuild without warning/error
				log.Println("Renaming workdir...")
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/ && mv package " + packageNameVersion
				fmt.Println(cmdStr)
				output, err = exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					log.Printf("error: %v", err)
					log.Println("Failed to rename workdir.")
					return
				}

				// Generate the dsc file
				log.Println("Signing the dsc file...")
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/" + packageNameVersion + " && dpkg-source --build . "
				fmt.Println(cmdStr)
				cmd := exec.Command("bash", "-c", cmdStr)
				// Make it interactive
				cmd.Stdout = os.Stdout
				cmd.Stdin = os.Stdin
				cmd.Stderr = os.Stderr
				err = cmd.Run()
				if err != nil {
					log.Printf("error: %v", err)
					log.Println("Failed to sign the package. Either you've the wrong key or you've unmeet dependencies. Please the error message(s) above..")
					return
				}

				// Signing the dsc file
				log.Println("Signing the dsc file...")
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/ && debsign -k" + maintainerSigningKey + " *.dsc"
				fmt.Println(cmdStr)
				cmd = exec.Command("bash", "-c", cmdStr)
				// Make it interactive
				cmd.Stdout = os.Stdout
				cmd.Stdin = os.Stdin
				cmd.Stderr = os.Stderr
				err = cmd.Run()
				if err != nil {
					log.Printf("error: %v", err)
					log.Println("Failed to sign the package. Either you've the wrong key or you've unmeet dependencies. Please the error message(s) above..")
					return
				}

				log.Println("Generate buildinfo file...")
				// Generate the buildinfo file
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/" + packageNameVersion + " && dpkg-genbuildinfo "
				fmt.Println(cmdStr)
				cmd = exec.Command("bash", "-c", cmdStr)
				// Make it interactive
				cmd.Stdout = os.Stdout
				cmd.Stdin = os.Stdin
				var buffer bytes.Buffer
				bufWriter := bufio.NewWriter(&buffer)
				cmd.Stderr = bufWriter
				err = cmd.Run()
				if err != nil && !strings.Contains(buffer.String(), ".buildinfo is meaningless") {
					log.Printf("error: %v", err)
					log.Println("Failed to sign the package. Either you've the wrong key or you've unmeet dependencies. Please the error message(s) above..")
					return
				}
				err = nil

				// Generate the changes file
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/" + packageNameVersion + " && dpkg-genchanges > ../$(ls .. | grep dsc | tr -d \".dsc\")_source.changes "
				fmt.Println(cmdStr)
				cmd = exec.Command("bash", "-c", cmdStr)
				// Make it interactive
				cmd.Stdout = os.Stdout
				cmd.Stdin = os.Stdin
				cmd.Stderr = os.Stderr
				err = cmd.Run()
				if err != nil {
					log.Printf("error: %v", err)
					log.Println("Failed to sign the package. Either you've the wrong key or you've unmeet dependencies. Please the error message(s) above..")
					return
				}

				// Lintian
				log.Println("Lintian test...")
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/" + packageNameVersion + " && lintian --profile blankon 2>&1"
				fmt.Println(cmdStr)
				output, err = exec.Command("bash", "-c", cmdStr).Output()
				log.Println(string(output)) // Print warnings as well
				// There is --fail-on error option on newer lintian version,
				// but let's just check the existence of "E:" string on output to determine error
				// to achieve backward compatibility with older lintian
				if !ignoreChecks && (err != nil || strings.Contains(string(output), "E:")) {
					log.Println("Failed to pass lintian.")
					return
				}

				log.Println("Rename move generated files to signed dir")
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += " && mkdir signed"
				cmdStr += " && mv *.xz ./signed/ || true " // ignore if err, native package has no orig tarball
				cmdStr += " && mv *.dsc ./signed/ "
				cmdStr += " && mv *.changes ./signed/ "
				err = exec.Command("bash", "-c", cmdStr).Run()
				if err != nil {
					log.Println(err)
					return
				}

				// Clean up
				log.Println("Cleaning up...")
				cmdStr = "rm -rf " + homeDir + "/.irgsh/tmp/" + tmpID + "/package"
				err = exec.Command("bash", "-c", cmdStr).Run()
				if err != nil {
					log.Println(err)
					return
				}

				// Compressing
				log.Println("Compressing...")
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += " && tar -zcvf ../" + tmpID + ".tar.gz ."
				err = exec.Command("bash", "-c", cmdStr).Run()
				if err != nil {
					log.Println(err)
					return err
				}

				submission := Submission{
					PackageName:            packageName,
					PackageVersion:         packageVersion,
					PackageExtendedVersion: packageExtendedVersion,
					PackageURL:             packageUrl,
					SourceURL:              sourceUrl,
					Maintainer:             maintainerIdentity,
					MaintainerFingerprint:  maintainerSigningKey,
					Component:              component,
					IsExperimental:         isExperimental,
					PackageBranch:          packageBranch,
					SourceBranch:           sourceBranch,
				}
				jsonByte, _ := json.Marshal(submission)

				// Signing a token
				log.Println("Signing auth token...")
				cmdStr = "cd " + homeDir + "/.irgsh/tmp/" + tmpID
				cmdStr += "/ && echo '" + b64.StdEncoding.EncodeToString(jsonByte) + "' | base64 -d > token && gpg -u " + maintainerSigningKey + " --clearsign --output token.sig --sign token"
				fmt.Println(cmdStr)
				cmd = exec.Command("bash", "-c", cmdStr)
				// Make it interactive
				cmd.Stdout = os.Stdout
				cmd.Stdin = os.Stdin
				cmd.Stderr = os.Stderr
				err = cmd.Run()
				if err != nil {
					log.Printf("error: %v", err)
					log.Println("Failed to sign the auth token using " + maintainerSigningKey + ". Please check your GPG key list.")
					return
				}

				// Upload
				log.Println("Uploading blob...")
				cmdStr = "curl -f -s --show-error -F 'blob=@" + homeDir + "/.irgsh/tmp/" + tmpID + ".tar.gz" + "' "
				cmdStr += " -F 'token=@" + homeDir + "/.irgsh/tmp/" + tmpID + "/token.sig" + "'"
				cmdStr += " '" + chiefAddress + "/api/v1/submission-upload' 2>&1"
				log.Println(cmdStr)
				output, err = exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					log.Println(string(output))
					return err
				}

				blobStr := strings.TrimSuffix(string(output), "\n")
				type Blob struct {
					ID string `json:"id"`
				}
				blob := Blob{}
				err = json.Unmarshal([]byte(blobStr), &blob)
				if err != nil {
					log.Println(err)
					return err
				}

				submission.Tarball = blob.ID
				jsonByte, _ = json.Marshal(submission)

				log.Println("Submitting...")
				result, err = req.Post(chiefAddress+"/api/v1/submit", header, req.BodyJSON(string(jsonByte)))
				if err != nil {
					log.Println(err)
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
					log.Println(err)
					return
				}
				fmt.Println("Submission succeeded. Pipeline ID:")
				fmt.Println(submissionResponse.PipelineID)
				cmdStr = "mkdir -p " + homeDir + "/.irgsh/tmp && echo -n '"
				cmdStr += submissionResponse.PipelineID + "' > " + homeDir + "/.irgsh/LAST_PIPELINE_ID"
				cmd = exec.Command("bash", "-c", cmdStr)
				err = cmd.Run()
				if err != nil {
					log.Println(err)
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
					log.Println(err.Error())
					return err
				}
				logResult := fmt.Sprintf("%+v", result)
				if strings.Contains(logResult, "404 page not found") {
					err = errors.New("Builder log is not found. The worker/pipeline may terminated ungracefully.")
					return err
				}
				fmt.Println(logResult)

				result, err = req.Get(chiefAddress+"/logs/"+pipelineId+".repo.log", nil)
				if err != nil {
					log.Println(err.Error())
					return err
				}
				logResult = fmt.Sprintf("%+v", result)
				if strings.Contains(logResult, "404 page not found") {
					err = errors.New("Repo log is not found. The worker/pipeline may terminated ungracefully.")
					return err
				}
				fmt.Println(logResult)

				return
			},
		},
		{
			Name:  "update",
			Usage: "Update the irgsh-cli tool",
			Action: func(c *cli.Context) (err error) {
				var (
					cmdStr          = "ln -sf /usr/bin/irgsh-cli /usr/bin/irgsh && /usr/bin/irgsh-cli --version"
					downloadURL     string
					githubResponse  GithubReleaseResponse
					githubAssetName = "irgsh-cli"
					url             = "https://api.github.com/repos/BlankOn/irgsh-go/releases/latest"
				)

				response, err := http.Get(url)
				if err != nil {
					log.Printf("error: %v\n", err)
					log.Println("Failed to get package name.")

					return
				}
				defer response.Body.Close()

				body, err := ioutil.ReadAll(response.Body)
				if err != nil {
					log.Printf("error: %v\n", err)

					return
				}

				if err := json.Unmarshal(body, &githubResponse); err != nil {
					log.Printf("error: %v\n", err)

					return err
				}

				for _, asset := range githubResponse.Assets {
					if asset.Name == githubAssetName {
						downloadURL = strings.TrimSuffix(string(asset.BrowserDownloadUrl), "\n")
						break
					}
				}

				log.Println(downloadURL)
				log.Println("Self-updating...")

				resp, err := http.Get(downloadURL)
				if err != nil {
					log.Printf("error: %v\n", err)

					return err
				}

				defer resp.Body.Close()

				err = update.Apply(resp.Body, update.Options{})
				if err != nil {
					log.Printf("error: %v\n", err)

					return err
				}

				log.Println(cmdStr)

				output, err := exec.Command("bash", "-c", cmdStr).Output()
				if err != nil {
					log.Println(output)
					log.Printf("error: %v\n", err)
					log.Println("Failed to get package name.")
				}
				log.Println("Updated to " + strings.TrimSuffix(string(output), "\n"))

				return
			},
		},
	}

	err = app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
