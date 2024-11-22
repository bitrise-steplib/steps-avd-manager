package emuinstaller

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"time"

	v1command "github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/hashicorp/go-retryablehttp"
)

type EmuInstaller struct {
	androidHome string
	cmdFactory  command.Factory
	logger      log.Logger
	httpClient  *retryablehttp.Client
}

const backupDir = "emulator_original"
const outputBuildIdRegex = "\\(build_id (\\d+)\\)"

func NewEmuInstaller(androidHome string, cmdFactory command.Factory, logger log.Logger, httpClient *retryablehttp.Client) EmuInstaller {
	return EmuInstaller{androidHome: androidHome, cmdFactory: cmdFactory, logger: logger, httpClient: httpClient}
}

func (e EmuInstaller) Install(buildNumber string) error {
	_, err := strconv.Atoi(buildNumber)
	if err != nil {
		return fmt.Errorf("the provided build number (%s) is not a number. Did you use the VERSION number instead of the BUILD number maybe?", buildNumber)
	}

	startTime := time.Now()

	installed, err := e.isVersionInstalled(buildNumber)
	if err != nil {
		e.logger.Warnf("Error checking if emulator build %s is installed: %w", buildNumber, err)
		// Assume not installed and continue
	}
	if installed {
		e.logger.Donef("Emulator build %s is already installed", buildNumber)
		return nil
	}

	err = e.backupEmuDir()
	if err != nil {
		return err
	}

	e.logger.Println()
	e.logger.Printf("Downloading emulator build %s...", buildNumber)
	err = e.download(buildNumber)
	if err != nil {
		return err
	}
	e.logger.Printf("Duration: %s", time.Since(startTime).Round(time.Second))

	isInstalled, err := e.isVersionInstalled(buildNumber)
	if err != nil {
		return fmt.Errorf("check if version is correct after install: %w", err)
	}
	if !isInstalled {
		return fmt.Errorf("version mismatch after install")
	}

	e.logger.Println()
	return nil
}

func (e EmuInstaller) isVersionInstalled(buildNumber string) (bool, error) {
	emuBinPath := filepath.Join(e.androidHome, "emulator", "emulator")
	versionCmd := e.cmdFactory.Create(emuBinPath, []string{"-version"}, nil)
	versionOut, err := versionCmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return false, fmt.Errorf("check emulator version: %w, output: %s", err, versionOut)
	}

	matches := regexp.MustCompile(outputBuildIdRegex).FindStringSubmatch(versionOut)
	if len(matches) < 2 {
		return false, fmt.Errorf("build number not found in emulator version output: %s", versionOut)
	}

	detectedBuildNumber := matches[1]
	return detectedBuildNumber == buildNumber, nil
}

func (e EmuInstaller) backupEmuDir() error {
	backupPath := filepath.Join(e.androidHome, backupDir)
	err := os.RemoveAll(backupPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove existing emulator backup at %s: %w", backupPath, err)
		}
	}

	_, err = os.Stat(filepath.Join(e.androidHome, "emulator"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Nothing to backup
			return nil
		}
		return fmt.Errorf("check if emulator exists: %w", err)
	}
	// https://stackoverflow.com/questions/73981482/moving-a-file-in-a-container-to-a-folder-that-has-a-mounted-volume-docker
	out, err := e.cmdFactory.Create(
		"mv",
		[]string{filepath.Join(e.androidHome, "emulator"), backupPath},
		nil,
	).RunAndReturnTrimmedCombinedOutput()

	if err != nil {
		return fmt.Errorf("backup existing emulator: %s", out)
	}

	return nil
}

func (e EmuInstaller) download(buildNumber string) error {
	goos := runtime.GOOS
	var arch string
	goarch := runtime.GOARCH
	switch goarch {
	case "amd64":
		arch = "x64"
	case "arm":
		arch = "aarch64"
	default:
		return fmt.Errorf("unsupported architecture %s", goarch)
	}

	url := downloadURL(goos, arch, buildNumber)

	downloadDir, err := os.MkdirTemp("", "emulator")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	zipPath := filepath.Join(downloadDir, "emulator.zip")

	resp, err := e.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("download emulator from %s: %w", url, err)
	}
	defer resp.Body.Close()

	file, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("create file %s: %w", zipPath, err)
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("download %s to %s: %w", url, zipPath, err)
	}

	err = v1command.UnZIP(zipPath, e.androidHome)
	if err != nil {
		return fmt.Errorf("unzip emulator: %w", err)
	}

	return nil
}

func downloadURL(os, arch, buildNumber string) string {
	// https://developer.android.com/studio/emulator_archive
	return fmt.Sprintf("https://redirector.gvt1.com/edgedl/android/repository/emulator-%s_%s-%s.zip", os, arch, buildNumber)
}
