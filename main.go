package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bitrise-io/go-android/sdk"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
	"github.com/kballard/go-shellquote"
)

// config ...
type config struct {
	AndroidHome       string `env:"ANDROID_HOME"`
	AndroidSDKRoot    string `env:"ANDROID_SDK_ROOT"`
	APILevel          int    `env:"api_level,required"`
	Tag               string `env:"tag,opt[google_apis,google_apis_playstore,android-wear,android-tv,default]"`
	DeviceProfile     string `env:"profile,required"`
	CreateCommandArgs string `env:"create_command_flags"`
	StartCommandArgs  string `env:"start_command_flags"`
	ID                string `env:"emulator_id,required"`
	Abi               string `env:"abi,opt[x86,armeabi-v7a,arm64-v8a,x86_64]"`
	EmulatorChannel   string `env:"emulator_channel,opt[0,1,2,3]"`
}

func runningDeviceInfos(androidHome string) (map[string]string, error) {
	cmd := command.New(filepath.Join(androidHome, "platform-tools", "adb"), "devices")
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return map[string]string{}, fmt.Errorf("command failed, error: %s", err)
	}

	log.Debugf("$ %s", cmd.PrintableCommandArgs())
	log.Debugf("%s", out)

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

func queryNewDeviceSerial(androidHome string, runningDevices map[string]string) (string, error) {
	currentRunningDevices, err := runningDeviceInfos(androidHome)
	if err != nil {
		return "", fmt.Errorf("failed to check running devices: %s", err)
	}

	serial := currentlyStartedDeviceSerial(runningDevices, currentRunningDevices)

	return serial, nil
}

type phase struct {
	name           string
	command        *command.Model
	customExecutor func(cmd *command.Model) func() (string, error)
}

func main() {
	var cfg config
	if err := stepconf.Parse(&cfg); err != nil {
		failf("Issue with input: %s", err)
	}
	stepconf.Print(cfg)
	fmt.Println()

	// Initialize Android SDK
	log.Printf("Initialize Android SDK")
	androidSdk, err := sdk.NewDefaultModel(sdk.Environment{
		AndroidHome:    cfg.AndroidHome,
		AndroidSDKRoot: cfg.AndroidSDKRoot,
	})
	if err != nil {
		failf("Failed to initialize Android SDK: %s", err)
	}

	androidHome := androidSdk.GetAndroidHome()
	runningDevices, err := runningDeviceInfos(androidHome)
	if err != nil {
		failf("Failed to check running devices, error: %s", err)
	}

	cmdlineToolsPath, err := androidSdk.CmdlineToolsPath()
	if err != nil {
		failf("Could not locate Android command-line tools: %v", err)
	}

	var (
		sdkManagerPath = filepath.Join(cmdlineToolsPath, "sdkmanager")
		avdManagerPath = filepath.Join(cmdlineToolsPath, "avdmanager")
		emulatorPath   = filepath.Join(androidHome, "emulator", "emulator")

		pkg     = fmt.Sprintf("system-images;android-%d;%s;%s", cfg.APILevel, cfg.Tag, cfg.Abi)
		yes, no = strings.Repeat("yes\n", 20), strings.Repeat("no\n", 20)
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
		{"Updating emulator",
			command.New(sdkManagerPath, "--verbose", "--channel="+cfg.EmulatorChannel, "emulator").
				SetStdin(strings.NewReader(yes)), // hitting yes in case it waits for accepting license
			nil,
		},

		{"Updating system-image packages",
			command.New(sdkManagerPath, "--verbose", pkg).
				SetStdin(strings.NewReader(yes)), // hitting yes in case it waits for accepting license
			nil,
		},

		{"Creating device",
			command.New(avdManagerPath, append([]string{
				"--verbose", "create", "avd", "--force",
				"--name", cfg.ID,
				"--device", cfg.DeviceProfile,
				"--package", pkg,
				"--tag", cfg.Tag,
				"--abi", cfg.Abi}, createCustomFlags...)...).
				SetStdin(strings.NewReader(no)), // hitting no in case it asks for creating hw profile
			nil,
		},
	} {
		log.Infof(phase.name)
		log.Donef("$ %s", phase.command.PrintableCommandArgs())

		var exec = phase.command.RunAndReturnTrimmedCombinedOutput
		if e := phase.customExecutor; e != nil {
			exec = e(phase.command)
		}

		if out, err := exec(); err != nil {
			failf("Failed to run phase: %s, output: %s", err, out)
		}

		fmt.Println()
	}

	var output bytes.Buffer
	deviceStartCmd := command.New(emulatorPath, append([]string{
		"@" + cfg.ID,
		"-verbose",
		"-show-kernel",
		"-no-audio",
		"-no-window",
		"-no-boot-anim",
		"-netdelay", "none",
		"-no-snapshot",
		"-wipe-data",
		"-gpu", "swiftshader_indirect"}, startCustomFlags...)...,
	).SetStdout(&output).SetStderr(&output)

	log.Infof("Starting device")
	log.Donef("$ %s", deviceStartCmd.PrintableCommandArgs())
	// start the emlator as a detached process
	emulatorWaitCh := make(chan error, 1)
	if err := deviceStartCmd.GetCmd().Start(); err != nil {
		failf("Failed to run device start command: %v", err)
	}
	go func() {
		emulatorWaitCh <- deviceStartCmd.GetCmd().Wait()
	}()

	var serial string
	const bootWaitTime = time.Duration(300)
	timeout := time.NewTimer(bootWaitTime * time.Second)
	deviceCheckTicker := time.NewTicker(5 * time.Second)

waitLoop:
	for {
		select {
		case err := <-emulatorWaitCh:
			log.Warnf("Emulator log: %s", output)
			if err != nil {
				failf("Emulator exited unexpectedly: %v", err)
			}
			failf("Emulator exited early, without error. A possible cause can be the emulator process having received a KILL signal.")
		case <-timeout.C:
			log.Warnf("Emulator log: %s", output)
			failf("Failed to boot emulator device within %d seconds.", bootWaitTime)
		case <-deviceCheckTicker.C:
			serial, err = queryNewDeviceSerial(androidHome, runningDevices)
			if err != nil {
				failf("Error: %s", err)
			} else if serial != "" {
				break waitLoop
			}
		}
	}
	timeout.Stop()
	deviceCheckTicker.Stop()

	if err := tools.ExportEnvironmentWithEnvman("BITRISE_EMULATOR_SERIAL", serial); err != nil {
		log.Warnf("Failed to export environment (BITRISE_EMULATOR_SERIAL), error: %s", err)
	}
	log.Printf("- Device with serial: %s started", serial)

	log.Donef("- Done")
}
