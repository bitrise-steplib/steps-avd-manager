package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bitrise-steplib/steps-avd-manager/avdconfig"
	"github.com/bitrise-tools/go-steputils/input"
	"github.com/bitrise-tools/go-steputils/tools"
	shellquote "github.com/kballard/go-shellquote"

	"github.com/bitrise-io/depman/pathutil"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
)

// ConfigsModel ...
type ConfigsModel struct {
	Version            string
	Resolution         string
	Orientation        string
	AndroidHome        string
	Tag                string
	ID                 string
	CustomConfig       string
	Density            string
	Overwrite          string
	CustomCommandFlags string
	Abi                string
	Profile            string
	Verbose            string
}

func createConfigsModelFromEnvs() ConfigsModel {
	return ConfigsModel{
		Version:            os.Getenv("version"),
		Resolution:         os.Getenv("resolution"),
		Orientation:        os.Getenv("orientation"),
		Tag:                os.Getenv("tag"),
		ID:                 os.Getenv("emulator_id"),
		Density:            os.Getenv("density"),
		Overwrite:          os.Getenv("overwrite"),
		CustomConfig:       os.Getenv("custom_hw_config"),
		AndroidHome:        os.Getenv("ANDROID_HOME"),
		CustomCommandFlags: os.Getenv("custom_command_flags"),
		Abi:                os.Getenv("emulator_abi"),
		Profile:            os.Getenv("profile"),
		Verbose:            os.Getenv("verbose_mode"),
	}
}

func (configs ConfigsModel) print() {
	log.Infof("Configs:")
	log.Printf("- Version: %s", configs.Version)
	log.Printf("- Resolution: %s", configs.Resolution)
	log.Printf("- Density: %s", configs.Density)
	log.Printf("- Orientation: %s", configs.Orientation)
	log.Printf("- Tag: %s", configs.Tag)
	log.Printf("- ABI: %s", configs.Abi)
	log.Printf("- Profile: %s", configs.Profile)
	log.Printf("- ID: %s", configs.ID)
	log.Printf("- CustomCommandFlags: %s", configs.CustomCommandFlags)
	log.Printf("- Overwrite: %s", configs.Overwrite)
	log.Printf("- CustomConfig: %s", configs.CustomConfig)
}

func (configs ConfigsModel) validate() error {
	if err := input.ValidateIfNotEmpty(configs.Version); err != nil {
		return fmt.Errorf("Version, %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.Overwrite); err != nil {
		return fmt.Errorf("Overwrite, %s", err)
	}
	if err := input.ValidateWithOptions(configs.Overwrite, "true", "false"); err != nil {
		return fmt.Errorf("Overwrite, %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.ID); err != nil {
		return fmt.Errorf("ID, %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.Orientation); err != nil {
		return fmt.Errorf("Orientation, %s", err)
	}
	if err := input.ValidateWithOptions(configs.Orientation, "portrait", "landscape"); err != nil {
		return fmt.Errorf("Orientation, %s", err)
	}
	if err := input.ValidateIfNotEmpty(configs.Abi); err != nil {
		return fmt.Errorf("Abi is not set")
	}
	if err := input.ValidateIfNotEmpty(configs.Profile); err != nil {
		return fmt.Errorf("Profile is not set")
	}
	if err := input.ValidateIfNotEmpty(configs.AndroidHome); err != nil {
		return fmt.Errorf("ANDROID_HOME is not set")
	}
	if err := input.ValidateIfPathExists(configs.AndroidHome); err != nil {
		return fmt.Errorf("ANDROID_HOME does not exists")
	}
	if err := input.ValidateIfNotEmpty(configs.Tag); err != nil {
		return fmt.Errorf("Tag, %s", err)
	}
	if err := input.ValidateWithOptions(configs.Tag, "google_apis", "google_apis_playstore", "android-wear", "android-tv", "default"); err != nil {
		return fmt.Errorf("Tag, %s", err)
	}
	if err := input.ValidateWithOptions(configs.Abi, "x86", "armeabi-v7a", "arm64-v8a", "x86_64", "mips"); err != nil {
		return fmt.Errorf("Abi, %s", err)
	}
	return nil
}

func main() {
	// Input validation
	configs := createConfigsModelFromEnvs()

	fmt.Println()
	configs.print()

	if err := configs.validate(); err != nil {
		fmt.Println()
		log.Errorf("Issue with input: %s", err)
		os.Exit(1)
	}

	fmt.Println()

	// update ensure the new sdkmanager, avdmanager
	{
		requiredSDKPackages := []string{"emulator", "tools", "platform-tools", fmt.Sprintf("system-images;android-%s;%s;%s", configs.Version, configs.Tag, configs.Abi)}

		log.Infof("Ensure sdk packages: %v", requiredSDKPackages)

		out, err := command.New(filepath.Join(configs.AndroidHome, "tools/bin/sdkmanager"), requiredSDKPackages...).RunAndReturnTrimmedCombinedOutput()
		if err != nil {
			failf("Failed to update emulator sdk package, error: %s, output: %s", err, out)
		}

		log.Donef("- Done")
	}

	avdPath := filepath.Join(os.Getenv("HOME"), ".android/avd", fmt.Sprintf("%s.avd", configs.ID))
	avdPathExists, err := pathutil.IsPathExists(avdPath)
	if err != nil {
		log.Errorf("Failed to check if path exists: %s", err)
		os.Exit(1)
	}

	iniPath := filepath.Join(os.Getenv("HOME"), ".android/avd", fmt.Sprintf("%s.ini", configs.ID))
	iniPathExists, err := pathutil.IsPathExists(iniPath)
	if err != nil {
		log.Errorf("Failed to check if path exists: %s", err)
		os.Exit(1)
	}

	if configs.Overwrite == "true" {
		if iniPathExists || avdPathExists {
			fmt.Println()
			log.Infof("Delete AVD")
			if avdPathExists {
				if err := os.RemoveAll(avdPath); err != nil {
					log.Errorf("Failed to remove avd dir: %s", err)
					os.Exit(1)
				}
			}
			if iniPathExists {
				if err := os.RemoveAll(iniPath); err != nil {
					log.Errorf("Failed to remove ini file: %s", err)
					os.Exit(1)
				}
			}
			log.Donef("- Done")
		}
	}

	// create emulator
	{
		if (!iniPathExists && !avdPathExists) || configs.Overwrite == "true" {
			fmt.Println()
			log.Infof("Create AVD")

			customProperties, err := avdconfig.NewProperties(strings.Split(configs.CustomConfig, "\n"))
			if err != nil {
				failf("Failed to parse custom properties, error: %s", err)
			}

			// -c 100M
			cmd := command.New(filepath.Join(configs.AndroidHome, "tools/bin/avdmanager"), "create", "avd", "-f",
				"-n", configs.ID,
				"-b", configs.Abi,
				"-g", configs.Tag,
				"-d", configs.Profile,
				"-c", customProperties.Get("sdcard.size", "128M"),
				"-k", fmt.Sprintf("system-images;android-%s;%s;%s", configs.Version, configs.Tag, configs.Abi))

			if out, err := cmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
				failf("Failed to create avd, error: %s output: %s", err, out)
			}

			avdConfig, err := avdconfig.Parse(filepath.Join(os.Getenv("HOME"), fmt.Sprintf(".android/avd/%s.avd/config.ini", configs.ID)))
			if err != nil {
				failf("Failed to parse config properties, error: %s", err)
			}

			if configs.Resolution != "" {
				avdConfig.Properties.Apply("skin.name", configs.Resolution)
			}
			if configs.Density != "" {
				avdConfig.Properties.Apply("hw.lcd.density", configs.Density)
			}
			avdConfig.Properties.Apply("PlayStore.enabled", fmt.Sprintf("%t", configs.Tag == "google_apis_playstore"))
			avdConfig.Properties.Apply("hw.initialOrientation", strings.Title(configs.Orientation))

			avdConfig.Properties.Append(customProperties)

			// ensure width and height are matching orientation
			{
				if avdConfig.Properties.Get("skin.name") != "" {
					res, err := ensureResolutionOrientation(configs.Resolution, configs.Orientation)
					if err != nil {
						failf("Failed to ensure device resolution, error: %s", err)
					}

					avdConfig.Properties.Apply("skin.name", res)
				}
			}

			if err := avdConfig.Save(); err != nil {
				failf("Failed to save avd config, error: %s", err)
			}

			log.Donef("- Done")
		} else {
			fmt.Println()
			log.Donef("Using existing AVD")
		}
	}

	// get currently running devices
	runningDevices, err := runningDeviceInfos(configs.AndroidHome)
	if err != nil {
		failf("Failed to check running devices, error: %s", err)
	}

	fmt.Println()

	// run emulator
	{
		log.Infof("Start emulator")

		customFlags, err := shellquote.Split(configs.CustomCommandFlags)
		if err != nil {
			log.Errorf("Failed to parse commands, error: %s", err)
			os.Exit(1)
		}

		cmdSlice := []string{"-avd", configs.ID}

		cmdSlice = append(cmdSlice, customFlags...)

		cmd := command.New(filepath.Join(configs.AndroidHome, "emulator/emulator"), cmdSlice...)

		osCommand := cmd.GetCmd()

		if configs.Verbose == "true" {
			osCommand.Stderr = os.Stderr
			osCommand.Stdout = os.Stdout
		}

		err = osCommand.Start()
		if err != nil {
			failf("Failed to start emulator, error: %s", err)
		}

		deviceDetectionStarted := time.Now()
		for true {
			time.Sleep(5 * time.Second)
			if osCommand.ProcessState != nil && osCommand.ProcessState.Exited() {
				failf("Emulator exited, error: %s", err)
			}

			currentRunningDevices, err := runningDeviceInfos(configs.AndroidHome)
			if err != nil {
				failf("Failed to check running devices, error: %s", err)
			}

			serial := currentlyStartedDeviceSerial(runningDevices, currentRunningDevices)

			if serial != "" {
				if err := tools.ExportEnvironmentWithEnvman("BITRISE_EMULATOR_SERIAL", serial); err != nil {
					log.Warnf("Failed to export environment (BITRISE_EMULATOR_SERIAL), error: %s", err)
				}
				log.Printf("- Device with serial: %s started", serial)
				break
			}

			bootWaitTime := time.Duration(300)

			if time.Now().After(deviceDetectionStarted.Add(bootWaitTime * time.Second)) {
				failf("Failed to boot emulator device within %d seconds.", bootWaitTime)
			}
		}

		log.Donef("- Done")
	}
}

func runningDeviceInfos(androidHome string) (map[string]string, error) {
	cmd := command.New(filepath.Join(androidHome, "platform-tools/adb"), "devices")
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return map[string]string{}, fmt.Errorf("command failed, error: %s", err)
	}

	// List of devices attached
	// emulator-5554	device
	deviceListItemPattern := `^(?P<emulator>emulator-\d*)[\s+](?P<state>.*)`
	deviceListItemRegexp := regexp.MustCompile(deviceListItemPattern)

	deviceStateMap := map[string]string{}

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		matches := deviceListItemRegexp.FindStringSubmatch(line)
		if len(matches) == 3 {
			serial := matches[1]
			state := matches[2]

			deviceStateMap[serial] = state
		}

	}
	if scanner.Err() != nil {
		return map[string]string{}, fmt.Errorf("scanner failed, error: %s", err)
	}

	return deviceStateMap, nil
}

func failf(msg string, args ...interface{}) {
	log.Errorf(msg, args...)
	os.Exit(1)
}

func currentlyStartedDeviceSerial(alreadyRunningDeviceInfos, currentlyRunningDeviceInfos map[string]string) string {
	startedSerial := ""

	for serial := range currentlyRunningDeviceInfos {
		_, found := alreadyRunningDeviceInfos[serial]
		if !found {
			startedSerial = serial
			break
		}
	}

	if len(startedSerial) > 0 {
		state := currentlyRunningDeviceInfos[startedSerial]
		if state == "device" {
			return startedSerial
		}
	}

	return ""
}

func ensureResolutionOrientation(res, orientation string) (string, error) {
	sides := strings.Split(res, "x")

	if len(sides) != 2 {
		return "", fmt.Errorf("Invalid resolution format: %s", res)
	}

	a, err := strconv.Atoi(sides[0])
	if err != nil {
		return "", err
	}

	b, err := strconv.Atoi(sides[1])
	if err != nil {
		return "", err
	}

	if strings.ToLower(orientation) == "portrait" {
		if a < b {
			return fmt.Sprintf("%dx%d", a, b), nil
		}
		return fmt.Sprintf("%dx%d", b, a), nil
	}

	if a > b {
		return fmt.Sprintf("%dx%d", a, b), nil
	}
	return fmt.Sprintf("%dx%d", b, a), nil
}
