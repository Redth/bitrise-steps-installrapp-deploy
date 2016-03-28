package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// -----------------------
// --- Constants
// -----------------------

const (
	installrAppDeployStatusKey     = "INSTALLRAPP_DEPLOY_STATUS"
	installrAppDeployStatusSuccess = "success"
	installrAppDeployStatusFailed  = "failed"
	installrAppDeployBuildURLKey   = "INSTALLRAPP_DEPLOY_BUILD_URL"
	installrAppDeployJson          = "INSTALLRAPP_DEPLOY_JSON"
)

// -----------------------
// --- Models
// -----------------------
type InstallrAppResponse struct {
    Action string
    Result string
    Message string
    AppData struct {
        AppId string
        AutoSync bool
        Id uint32
        Title string
        LatestBuild struct {
            Id uint32
            BuildFile struct {
                BuildSize string
                Url string
            }
            DateCreated string
            Icon struct {
                BuildSize string
                Url string
            }
        }
    }
}

// -----------------------
// --- Functions
// -----------------------

func logFail(format string, v ...interface{}) {
	if err := exportEnvironmentWithEnvman(installrAppDeployStatusKey, installrAppDeployStatusFailed); err != nil {
		logWarn("Failed to export %s, error: %s", installrAppDeployStatusKey, err)
	}

	errorMsg := fmt.Sprintf(format, v...)
	fmt.Printf("\x1b[31;1m%s\x1b[0m\n", errorMsg)
	os.Exit(1)
}

func logWarn(format string, v ...interface{}) {
	errorMsg := fmt.Sprintf(format, v...)
	fmt.Printf("\x1b[33;1m%s\x1b[0m\n", errorMsg)
}

func logInfo(format string, v ...interface{}) {
	fmt.Println()
	errorMsg := fmt.Sprintf(format, v...)
	fmt.Printf("\x1b[34;1m%s\x1b[0m\n", errorMsg)
}

func logDetails(format string, v ...interface{}) {
	errorMsg := fmt.Sprintf(format, v...)
	fmt.Printf("  %s\n", errorMsg)
}

func logDone(format string, v ...interface{}) {
	errorMsg := fmt.Sprintf(format, v...)
	fmt.Printf("  \x1b[32;1m%s\x1b[0m\n", errorMsg)
}

func genericIsPathExists(pth string) (os.FileInfo, bool, error) {
	if pth == "" {
		return nil, false, errors.New("No path provided")
	}
	fileInf, err := os.Stat(pth)
	if err == nil {
		return fileInf, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return fileInf, false, err
}

// IsDirExists ...
func IsDirExists(pth string) (bool, error) {
	fileInf, isExists, err := genericIsPathExists(pth)
	if err != nil {
		return false, err
	}
	if !isExists {
		return false, nil
	}
	if fileInf == nil {
		return false, errors.New("No file info available.")
	}
	return fileInf.IsDir(), nil
}

// IsPathExists ...
func IsPathExists(pth string) (bool, error) {
	_, isExists, err := genericIsPathExists(pth)
	return isExists, err
}

func exportEnvironmentWithEnvman(keyStr, valueStr string) error {
	envman := exec.Command("envman", "add", "--key", keyStr)
	envman.Stdin = strings.NewReader(valueStr)
	envman.Stdout = os.Stdout
	envman.Stderr = os.Stderr
	return envman.Run()
}

func createRequest(url string, fields, files map[string]string) (*http.Request, error) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	// Add fields
	for key, value := range fields {
		if err := w.WriteField(key, value); err != nil {
			return nil, err
		}
	}

	// Add files
	for key, file := range files {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		fw, err := w.CreateFormFile(key, file)
		if err != nil {
			return nil, err
		}
		if _, err = io.Copy(fw, f); err != nil {
			return nil, err
		}
	}

	w.Close()

	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", w.FormDataContentType())

	return req, nil
}

// -----------------------
// --- Main
// -----------------------

func main() {
	//
	// Validate options
	ipaPath := os.Getenv("ipa_path")
	apiToken := os.Getenv("api_token")
	notes := os.Getenv("notes")
	notify := os.Getenv("notify")
    add := os.Getenv("add")
	
	logInfo("Configs:")
	logDetails("ipa_path: %s", ipaPath)
	logDetails("api_token: ***")
	logDetails("releaseNotes: %s", notes)
	logDetails("notify: %s", notify)
    logDetails("add: %s", add)
	
	if ipaPath == "" {
		logFail("Missing required input: ipa_path")
	}
	if exist, err := IsPathExists(ipaPath); err != nil {
		logFail("Failed to check if path (%s) exist, error: %#v", ipaPath, err)
	} else if !exist {
		logFail("No IPA found to deploy. Specified path was: %s", ipaPath)
	}

	if apiToken == "" {
		logFail("No App api_token provided as environment variable. Terminating...")
	}

	//
	// Create request
	logInfo("Performing request")

	requestURL := "https://www.installrapp.com/apps.json"

	fields := map[string]string{
		"releaseNotes":     notes,
		"notify":           notify,
        "add":              add,
	}

	files := map[string]string{
		"qqfile": ipaPath,
	}
	
	request, err := createRequest(requestURL, fields, files)
	if err != nil {
		logFail("Failed to create request, error: %#v", err)
	}
	request.Header.Add("X-InstallrAppToken", apiToken)

	client := http.Client{}
	response, requestErr := client.Do(request)

	defer response.Body.Close()
	contents, readErr := ioutil.ReadAll(response.Body)

	//
	// Process response

	// Error
	if requestErr != nil {
		if readErr != nil {
			logWarn("Failed to read response body, error: %#v", readErr)
		} else {
			logInfo("Response:")
			logDetails("status code: %d", response.StatusCode)
			logDetails("body: %s", string(contents))
		}
		logFail("Performing request failed, error: %#v", requestErr)
	}

	if response.StatusCode < 200 || response.StatusCode > 300 {
		if readErr != nil {
			logWarn("Failed to read response body, error: %#v", readErr)
		} else {
			logInfo("Response:")
			logDetails("status code: %d", response.StatusCode)
			logDetails("body: %s", string(contents))
		}
		logFail("Performing request failed, status code: %d", response.StatusCode)
	}

	// Success
	logDone("Request succeeded")

	logInfo("Response:")
	logDetails("status code: %d", response.StatusCode)
	logDetails("body: %s", contents)

	if readErr != nil {
		logFail("Failed to read response body, error: %#v", readErr)
	}

    // Decode the json object
    iar := &InstallrAppResponse{}
    if err := json.Unmarshal([]byte(contents), &iar); err != nil {
    	logFail("Failed to parse response body, error: %#v", err)    
    }
    
    fmt.Println()
    
    // Defaults
    var responseResult = "failed"
    var responseBuildUrl = ""
    
    // See if our decoded object has the fields we want
    if (iar != nil) {
        responseResult = iar.Result        
        responseBuildUrl = iar.AppData.LatestBuild.BuildFile.Url
    }
    
    // Log some info
    logDone("Status: %s", responseResult)
    
	if responseBuildUrl != "" {
		logDone("Build URL: %s", responseBuildUrl)
	}
	
    // Export our variables
	if err := exportEnvironmentWithEnvman(installrAppDeployStatusKey, responseResult); err != nil {
		logFail("Failed to export %s, error: %#v", installrAppDeployStatusKey, err)
	}

	if err := exportEnvironmentWithEnvman(installrAppDeployBuildURLKey, responseBuildUrl); err != nil {
		logFail("Failed to export %s, error: %#v", installrAppDeployBuildURLKey, err)
	}

	if err := exportEnvironmentWithEnvman(installrAppDeployJson, string(contents)); err != nil {
		logFail("Failed to export %s, error: %#v", installrAppDeployJson, err)
	}
}
