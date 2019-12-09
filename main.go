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
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
	"github.com/kballard/go-shellquote"
)

// config ...
type config struct {
	AndroidHome       string `env:"ANDROID_HOME,required"`
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

	runningDevices, err := runningDeviceInfos(cfg.AndroidHome)
	if err != nil {
		failf("Failed to check running devices, error: %s", err)
	}

	var (
		sdkManagerPath = filepath.Join(cfg.AndroidHome, "tools/bin/sdkmanager")
		avdManagerPath = filepath.Join(cfg.AndroidHome, "tools/bin/avdmanager")
		emulatorPath   = filepath.Join(cfg.AndroidHome, "emulator/emulator")

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
		{"Update emulator",
			command.New(sdkManagerPath, "--verbose", "--channel="+cfg.EmulatorChannel, "emulator").
				SetStdin(strings.NewReader(yes)), // hitting yes in case it waits for accepting license
			nil,
		},

		{"Update system-image packages",
			command.New(sdkManagerPath, "--verbose", pkg).
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
				"--abi", cfg.Abi}, createCustomFlags...)...).
				SetStdin(strings.NewReader(no)), // hitting no in case it asks for creating hw profile
			nil,
		},

		{"Start device",
			command.New(emulatorPath, append([]string{
				"@" + cfg.ID,
				"-verbose",
				"-show-kernel",
				"-no-audio",
				"-no-window",
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
		log.Donef("$ %s", phase.command.PrintableCommandArgs())

		var exec = phase.command.RunAndReturnTrimmedCombinedOutput
		if e := phase.customExecutor; e != nil {
			exec = e(phase.command)
		}

		if out, err := exec(); err != nil {
			failf("Failed to run phase, error: %s, output: %s", err, out)
		}

		fmt.Println()
	}

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
}
