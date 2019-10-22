package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/kballard/go-shellquote"

	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
)

// config ...
type config struct {
	AndroidHome       string `env:"ANDROID_HOME,required"`
	APILevel          int    `env:"api_level,required"`
	Tag               string `env:"tag,required"`
	DeviceProfile     string `env:"profile,required"`
	CreateCommandArgs string `env:"create_command_flags"`
	StartCommandArgs  string `env:"start_command_flags"`
	Verbose           bool   `env:"verbose,required"`
	ID                string `env:"emulator_id,required"`
	// WaitForBoot       bool   `env:"wait_for_boot,required"`

	//Resolution         string
	//Orientation        string
	//CustomConfig       string
	//Density            string
	//Overwrite          string
	//Abi                string
	//Profile            string
}

// func createConfigsModelFromEnvs() ConfigsModel {
// 	return ConfigsModel{
// 		Version:            os.Getenv("version"),
// 		Resolution:         os.Getenv("resolution"),
// 		Orientation:        os.Getenv("orientation"),
// 		Tag:                os.Getenv("tag"),
// 		ID:                 os.Getenv("emulator_id"),
// 		Density:            os.Getenv("density"),
// 		Overwrite:          os.Getenv("overwrite"),
// 		CustomConfig:       os.Getenv("custom_hw_config"),
// 		AndroidHome:        os.Getenv("ANDROID_HOME"),
// 		CustomCommandFlags: os.Getenv("custom_command_flags"),
// 		Abi:                os.Getenv("emulator_abi"),
// 		Profile:            os.Getenv("profile"),
// 		Verbose:            os.Getenv("verbose_mode"),
// 	}
// }

// func (configs ConfigsModel) print() {
// 	log.Infof("Configs:")
// 	log.Printf("- Version: %s", configs.Version)
// 	log.Printf("- Resolution: %s", configs.Resolution)
// 	log.Printf("- Density: %s", configs.Density)
// 	log.Printf("- Orientation: %s", configs.Orientation)
// 	log.Printf("- Tag: %s", configs.Tag)
// 	log.Printf("- ABI: %s", configs.Abi)
// 	log.Printf("- Profile: %s", configs.Profile)
// 	log.Printf("- ID: %s", configs.ID)
// 	log.Printf("- CustomCommandFlags: %s", configs.CustomCommandFlags)
// 	log.Printf("- Overwrite: %s", configs.Overwrite)
// 	log.Printf("- CustomConfig:\n%s", configs.CustomConfig)
// }

// func (configs ConfigsModel) validate() error {
// 	if err := input.ValidateIfNotEmpty(configs.Version); err != nil {
// 		return fmt.Errorf("Version, %s", err)
// 	}
// 	if err := input.ValidateIfNotEmpty(configs.Overwrite); err != nil {
// 		return fmt.Errorf("Overwrite, %s", err)
// 	}
// 	if err := input.ValidateWithOptions(configs.Overwrite, "true", "false"); err != nil {
// 		return fmt.Errorf("Overwrite, %s", err)
// 	}
// 	if err := input.ValidateIfNotEmpty(configs.ID); err != nil {
// 		return fmt.Errorf("ID, %s", err)
// 	}
// 	if err := input.ValidateIfNotEmpty(configs.Orientation); err != nil {
// 		return fmt.Errorf("Orientation, %s", err)
// 	}
// 	if err := input.ValidateWithOptions(configs.Orientation, "portrait", "landscape"); err != nil {
// 		return fmt.Errorf("Orientation, %s", err)
// 	}
// 	if err := input.ValidateIfNotEmpty(configs.Abi); err != nil {
// 		return fmt.Errorf("Abi is not set")
// 	}
// 	if err := input.ValidateIfNotEmpty(configs.Profile); err != nil {
// 		return fmt.Errorf("Profile is not set")
// 	}
// 	if err := input.ValidateIfNotEmpty(configs.AndroidHome); err != nil {
// 		return fmt.Errorf("ANDROID_HOME is not set")
// 	}
// 	if err := input.ValidateIfPathExists(configs.AndroidHome); err != nil {
// 		return fmt.Errorf("ANDROID_HOME does not exists")
// 	}
// 	if err := input.ValidateIfNotEmpty(configs.Tag); err != nil {
// 		return fmt.Errorf("Tag, %s", err)
// 	}
// 	if err := input.ValidateWithOptions(configs.Tag, "google_apis", "google_apis_playstore", "android-wear", "android-tv", "default"); err != nil {
// 		return fmt.Errorf("Tag, %s", err)
// 	}
// 	if err := input.ValidateWithOptions(configs.Abi, "x86", "armeabi-v7a", "arm64-v8a", "x86_64", "mips"); err != nil {
// 		return fmt.Errorf("Abi, %s", err)
// 	}
// 	return nil
// }

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

// func ensureResolutionOrientation(res, orientation string) (string, error) {
// 	sides := strings.Split(res, "x")

// 	if len(sides) != 2 {
// 		return "", fmt.Errorf("invalid resolution format: %s", res)
// 	}

// 	a, err := strconv.Atoi(sides[0])
// 	if err != nil {
// 		return "", err
// 	}

// 	b, err := strconv.Atoi(sides[1])
// 	if err != nil {
// 		return "", err
// 	}

// 	if strings.ToLower(orientation) == "portrait" {
// 		if a < b {
// 			return fmt.Sprintf("%dx%d", a, b), nil
// 		}
// 		return fmt.Sprintf("%dx%d", b, a), nil
// 	}

// 	if a > b {
// 		return fmt.Sprintf("%dx%d", a, b), nil
// 	}
// 	return fmt.Sprintf("%dx%d", b, a), nil
// }

type phase struct {
	name           string
	command        *command.Model
	customExecutor func(cmd *command.Model) func() (string, error)
}

//rn commands:
//sudo $ANDROID_HOME/tools/bin/sdkmanager --update
//$ANDROID_HOME/tools/bin/sdkmanager --install "emulator"
//$ANDROID_HOME/tools/bin/sdkmanager --install "system-images;android-29;google_apis;x86"
//echo "no" | $ANDROID_HOME/tools/bin/avdmanager --verbose create avd --force --name "pixel" --device "pixel" --package "system-images;android-29;google_apis;x86" --tag "google_apis" --abi "x86"
//$ANDROID_HOME/emulator/emulator-headless  &> /tmp/log.txt &
//sleep 160

func main() {
	var cfg config
	if err := stepconf.Parse(&cfg); err != nil {
		failf("Issue with input: %s", err)
	}
	stepconf.Print(cfg)
	fmt.Println()

	runningDevices, err := runningDeviceInfos(cfg.AndroidHome)
	if err != nil {
		failf("Failed to check running devices, error: %s", err)
	}

	var (
		sdkManagerPath = filepath.Join(cfg.AndroidHome, "tools/bin/sdkmanager")
		avdManagerPath = filepath.Join(cfg.AndroidHome, "tools/bin/avdmanager")
		emulatorPath   = filepath.Join(cfg.AndroidHome, "emulator/emulator-headless")
		pkg            = fmt.Sprintf("system-images;android-%d;%s;x86", cfg.APILevel, cfg.Tag)
		yes, no        = strings.Repeat("yes\n", 20), strings.Repeat("no\n", 20)
	)

	// parse custom flags
	createCustomFlags, err := shellquote.Split(cfg.CreateCommandArgs)
	if err != nil {
		failf("Failed to parse create command args, error: %s", err)
	}
	startCustomFlags, err := shellquote.Split(cfg.StartCommandArgs)
	if err != nil {
		failf("Failed to parse start command args, error: %s", err)
	}

	for _, phase := range []phase{
		{"Update SDK Manager",
			command.New("sh", "-c", "sudo "+sdkManagerPath+" --verbose --update"),
			nil,
		},

		{"Update emulator and system-image packages",
			command.New(sdkManagerPath,
				"--verbose", "emulator", pkg).
				SetStdin(strings.NewReader(yes)), // hitting yes in case it waits for accepting license
			nil,
		},

		{"Create device",
			command.New(avdManagerPath, append([]string{
				"--verbose", "create", "avd", "--force",
				"--name", cfg.ID,
				"--device", cfg.DeviceProfile,
				"--package", pkg,
				"--tag", cfg.Tag,
				"--abi", "x86"}, createCustomFlags...)...).
				SetStdin(strings.NewReader(no)), // hitting no in case it asks for creating hw profile
			nil,
		},

		{"Start device",
			command.New(emulatorPath, append([]string{
				"@" + cfg.ID,
				"-verbose",
				"-show-kernel",
				"-no-audio",
				"-no-boot-anim",
				"-netdelay", "none",
				"-no-snapshot",
				"-wipe-data",
				"-gpu", "swiftshader_indirect"}, startCustomFlags...)...),
			func(cmd *command.Model) func() (string, error) { // need to start the emlator as a detached process
				return func() (string, error) {
					return "", cmd.GetCmd().Start()
				}
			},
		},
	} {
		log.Infof(phase.name)
		fmt.Println()
		log.Donef("$ %s", phase.command.PrintableCommandArgs())

		if cfg.Verbose {
			phase.command.SetStdout(os.Stdout)
		}

		var executor = phase.command.RunAndReturnTrimmedCombinedOutput
		if e := phase.customExecutor; e != nil {
			executor = e(phase.command)
		}

		if out, err := executor(); err != nil {
			failf("Failed to run phase, error: %s, output: %s", err, out)
		}

		fmt.Println()
	}
	// // Input validation
	// configs := createConfigsModelFromEnvs()

	// fmt.Println()
	// configs.print()

	// if err := configs.validate(); err != nil {
	// 	fmt.Println()
	// 	log.Errorf("Issue with input: %s", err)
	// 	os.Exit(1)
	// }

	// fmt.Println()

	// // update ensure the new sdkmanager, avdmanager
	// {
	// 	requiredSDKPackages := []string{"tools", "platform-tools", fmt.Sprintf("system-images;android-%s;%s;%s", configs.Version, configs.Tag, configs.Abi)}

	// 	log.Infof("Ensure sdk packages: %v", requiredSDKPackages)

	// 	out, err := command.New(filepath.Join(configs.AndroidHome, "tools/bin/sdkmanager"), requiredSDKPackages...).RunAndReturnTrimmedCombinedOutput()
	// 	if err != nil {
	// 		failf("Failed to update emulator sdk package, error: %s, output: %s", err, out)
	// 	}

	// 	// getting emulator from different channel
	// 	out, err = command.New(filepath.Join(configs.AndroidHome, "tools/bin/sdkmanager"), "emulator", "--channel=3").RunAndReturnTrimmedCombinedOutput()
	// 	if err != nil {
	// 		failf("Failed to update emulator sdk package, error: %s, output: %s", err, out)
	// 	}

	// 	log.Donef("- Done")
	// }

	// avdPath := filepath.Join(os.Getenv("HOME"), ".android/avd", fmt.Sprintf("%s.avd", configs.ID))
	// avdPathExists, err := pathutil.IsPathExists(avdPath)
	// if err != nil {
	// 	log.Errorf("Failed to check if path exists: %s", err)
	// 	os.Exit(1)
	// }

	// iniPath := filepath.Join(os.Getenv("HOME"), ".android/avd", fmt.Sprintf("%s.ini", configs.ID))
	// iniPathExists, err := pathutil.IsPathExists(iniPath)
	// if err != nil {
	// 	log.Errorf("Failed to check if path exists: %s", err)
	// 	os.Exit(1)
	// }

	// if configs.Overwrite == "true" {
	// 	if iniPathExists || avdPathExists {
	// 		fmt.Println()
	// 		log.Infof("Delete AVD")
	// 		if avdPathExists {
	// 			if err := os.RemoveAll(avdPath); err != nil {
	// 				log.Errorf("Failed to remove avd dir: %s", err)
	// 				os.Exit(1)
	// 			}
	// 		}
	// 		if iniPathExists {
	// 			if err := os.RemoveAll(iniPath); err != nil {
	// 				log.Errorf("Failed to remove ini file: %s", err)
	// 				os.Exit(1)
	// 			}
	// 		}
	// 		avdPathExists = false
	// 		iniPathExists = false
	// 		log.Donef("- Done")
	// 	}
	// }

	// // create emulator
	// {
	// 	if !iniPathExists || !avdPathExists {
	// 		fmt.Println()
	// 		log.Infof("Create AVD")

	// 		customProperties, err := avdconfig.NewProperties(strings.Split(configs.CustomConfig, "\n"))
	// 		if err != nil {
	// 			failf("Failed to parse custom properties, error: %s", err)
	// 		}

	// 		// -c 100M
	// 		cmd := command.New(filepath.Join(configs.AndroidHome, "tools/bin/avdmanager"), "create", "avd", "-f",
	// 			"-n", configs.ID,
	// 			"-b", configs.Abi,
	// 			"-g", configs.Tag,
	// 			"-d", configs.Profile,
	// 			"-c", customProperties.Get("sdcard.size", "128M"),
	// 			"-k", fmt.Sprintf("system-images;android-%s;%s;%s", configs.Version, configs.Tag, configs.Abi))

	// 		if out, err := cmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
	// 			failf("Failed to create avd, error: %s output: %s", err, out)
	// 		}

	// 		avdConfig, err := avdconfig.Parse(filepath.Join(os.Getenv("HOME"), fmt.Sprintf(".android/avd/%s.avd/config.ini", configs.ID)))
	// 		if err != nil {
	// 			failf("Failed to parse config properties, error: %s", err)
	// 		}

	// 		if configs.Resolution != "" {
	// 			avdConfig.Properties.Apply("skin.name", configs.Resolution)
	// 		} else {
	// 			if avdConfig.Properties.Get("skin.name", "") == "" {
	// 				width := avdConfig.Properties.Get("hw.lcd.width", "")
	// 				height := avdConfig.Properties.Get("hw.lcd.height", "")

	// 				if width != "" && height != "" {
	// 					avdConfig.Properties.Apply("skin.name", fmt.Sprintf("%sx%s", width, height))
	// 				}
	// 			}
	// 		}
	// 		if configs.Density != "" {
	// 			avdConfig.Properties.Apply("hw.lcd.density", configs.Density)
	// 		}
	// 		avdConfig.Properties.Apply("PlayStore.enabled", fmt.Sprintf("%t", configs.Tag == "google_apis_playstore"))
	// 		avdConfig.Properties.Apply("hw.initialOrientation", strings.Title(configs.Orientation))

	// 		avdConfig.Properties.Append(customProperties)

	// 		// ensure width and height are matching orientation
	// 		{
	// 			skin := avdConfig.Properties.Get("skin.name", "")
	// 			if skin != "" {
	// 				res, err := ensureResolutionOrientation(skin, configs.Orientation)
	// 				if err != nil {
	// 					failf("Failed to ensure device resolution, error: %s", err)
	// 				}

	// 				avdConfig.Properties.Apply("skin.name", res)
	// 			}
	// 		}

	// 		if err := avdConfig.Save(); err != nil {
	// 			failf("Failed to save avd config, error: %s", err)
	// 		}

	// 		log.Donef("- Done")
	// 	} else {
	// 		fmt.Println()
	// 		log.Donef("Using existing AVD")
	// 	}
	// }

	// // get currently running devices
	// runningDevices, err := runningDeviceInfos(configs.AndroidHome)
	// if err != nil {
	// 	failf("Failed to check running devices, error: %s", err)
	// }

	// fmt.Println()

	// // run emulator
	// {
	// 	log.Infof("Start emulator")

	// 	customFlags, err := shellquote.Split(configs.CustomCommandFlags)
	// 	if err != nil {
	// 		log.Errorf("Failed to parse commands, error: %s", err)
	// 		os.Exit(1)
	// 	}

	// 	cmdSlice := []string{"-avd", configs.ID}

	// 	cmdSlice = append(cmdSlice, customFlags...)

	// 	cmd := command.New(filepath.Join(configs.AndroidHome, "emulator/emulator"), cmdSlice...)

	// 	osCommand := cmd.GetCmd()

	// 	if configs.Verbose == "true" {
	// 		osCommand.Stderr = os.Stderr
	// 		osCommand.Stdout = os.Stdout
	// 	}

	// 	err = osCommand.Start()
	// 	if err != nil {
	// 		failf("Failed to start emulator, error: %s", err)
	// 	}

	deviceDetectionStarted := time.Now()
	for true {
		currentRunningDevices, err := runningDeviceInfos(cfg.AndroidHome)
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

		time.Sleep(5 * time.Second)
	}

	log.Donef("- Done")
	// }
}
