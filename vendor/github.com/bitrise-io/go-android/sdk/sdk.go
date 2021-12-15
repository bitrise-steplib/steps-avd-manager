package sdk

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/hashicorp/go-version"
)

// Model ...
type Model struct {
	androidHome string
}

// Environment is used to pass in environment variables used to locate Android SDK
type Environment struct {
	AndroidHome    string // ANDROID_HOME
	AndroidSDKRoot string // ANDROID_SDK_ROOT
}

// NewEnvironment gets needed environment variables
func NewEnvironment() *Environment {
	return &Environment{
		AndroidHome:    os.Getenv("ANDROID_HOME"),
		AndroidSDKRoot: os.Getenv("ANDROID_SDK_ROOT"),
	}
}

// AndroidSdkInterface ...
type AndroidSdkInterface interface {
	AndroidHome() string
	CmdlineTools() (string, error)
	BuildTools() (string, error)
	PlatformTools() string
}

// New creates a Model with a supplied Android SDK path
func New(androidHome string) (*Model, error) {
	evaluatedSDKRoot, err := validateAndroidSDKRoot(androidHome)
	if err != nil {
		return nil, err
	}

	return &Model{androidHome: evaluatedSDKRoot}, nil
}

// NewDefaultModel locates Android SDK based on environement variables
func NewDefaultModel(envs Environment) (*Model, error) {
	// https://developer.android.com/studio/command-line/variables#envar
	// Sets the path to the SDK installation directory.
	// ANDROID_HOME, which also points to the SDK installation directory, is deprecated.
	// If you continue to use it, the following rules apply:
	//  If ANDROID_HOME is defined and contains a valid SDK installation, its value is used instead of the value in ANDROID_SDK_ROOT.
	//  If ANDROID_HOME is not defined, the value in ANDROID_SDK_ROOT is used.
	var warnings []string
	for _, SDKdir := range []string{envs.AndroidHome, envs.AndroidSDKRoot} {
		if SDKdir == "" {
			warnings = append(warnings, "environment variable is unset or empty")
			continue
		}

		evaluatedSDKRoot, err := validateAndroidSDKRoot(SDKdir)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}

		return &Model{androidHome: evaluatedSDKRoot}, nil
	}

	return nil, fmt.Errorf("could not locate Android SDK root directory: %s", warnings)
}

func validateAndroidSDKRoot(androidSDKRoot string) (string, error) {
	evaluatedSDKRoot, err := filepath.EvalSymlinks(androidSDKRoot)
	if err != nil {
		return "", err
	}

	if exist, err := pathutil.IsDirExists(evaluatedSDKRoot); err != nil {
		return "", err
	} else if !exist {
		return "", fmt.Errorf("(%s) is not a valid Android SDK root", evaluatedSDKRoot)
	}

	return evaluatedSDKRoot, nil
}

// AndroidHome ...
func (model *Model) AndroidHome() string {
	return model.androidHome
}

// PlatformTools ...
func (model *Model) PlatformTools() string {
	return filepath.Join(model.AndroidHome(), "platform-tools")
}

// BuildTools ...
func (model *Model) BuildTools() (string, error) {
	buildTools := filepath.Join(model.AndroidHome(), "build-tools")
	pattern := filepath.Join(buildTools, "*")

	buildToolsDirs, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}

	var latestVersion *version.Version
	for _, buildToolsDir := range buildToolsDirs {
		versionStr := strings.TrimPrefix(buildToolsDir, buildTools+"/")
		version, err := version.NewVersion(versionStr)
		if err != nil {
			continue
		}

		if latestVersion == nil || version.GreaterThan(latestVersion) {
			latestVersion = version
		}
	}

	if latestVersion == nil || latestVersion.String() == "" {
		return "", errors.New("failed to find latest build-tools dir")
	}

	return filepath.Join(buildTools, latestVersion.String()), nil
}

// BuildTool ...
func (model *Model) BuildTool(name string) (string, error) {
	buildToolsDir, err := model.BuildTools()
	if err != nil {
		return "", err
	}

	pth := filepath.Join(buildToolsDir, name)
	if exist, err := pathutil.IsPathExists(pth); err != nil {
		return "", err
	} else if !exist {
		return "", fmt.Errorf("tool (%s) not found at: %s", name, buildToolsDir)
	}

	return pth, nil
}

// CmdlineTools locates the command-line tools directory
func (model *Model) CmdlineTools() (string, error) {
	toolPaths := []string{
		filepath.Join(model.AndroidHome(), "cmdline-tools", "latest", "bin"),
		filepath.Join(model.AndroidHome(), "cmdline-tools", "*", "bin"),
		filepath.Join(model.AndroidHome(), "tools", "bin"),
		filepath.Join(model.AndroidHome(), "tools"), // legacy
	}

	var warnings []string
	for _, dirPattern := range toolPaths {
		matches, err := filepath.Glob(dirPattern)
		if err != nil {
			return "", fmt.Errorf("failed to locate Android command-line tools directory, invalid patterns specified (%s): %s", toolPaths, err)
		}

		if len(matches) == 0 {
			continue
		}

		sdkmanagerPath := matches[0]
		if exists, err := pathutil.IsDirExists(sdkmanagerPath); err != nil {
			warnings = append(warnings, fmt.Sprintf("failed to validate path (%s): %v", sdkmanagerPath, err))
			continue
		} else if !exists {
			warnings = append(warnings, "path (%s) does not exist or it is not a directory")
			continue
		}

		return sdkmanagerPath, nil
	}

	return "", fmt.Errorf("failed to locate Android command-line tools directory on paths (%s), warnings: %s", toolPaths, warnings)
}
