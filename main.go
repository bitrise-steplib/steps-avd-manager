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
	"github.com/bitrise-io/go-utils/sliceutil"
	"github.com/bitrise-io/go-utils/v2/system"
	"github.com/kballard/go-shellquote"
)

// config ...
type config struct {
	AndroidHome          string `env:"ANDROID_HOME"`
	AndroidSDKRoot       string `env:"ANDROID_SDK_ROOT"`
	APILevel             int    `env:"api_level,required"`
	Tag                  string `env:"tag,opt[google_apis,google_apis_playstore,aosp_atd,google_atd,android-wear,android-tv,default]"`
	DeviceProfile        string `env:"profile,required"`
	CreateCommandArgs    string `env:"create_command_flags"`
	StartCommandArgs     string `env:"start_command_flags"`
	ID                   string `env:"emulator_id,required"`
	Abi                  string `env:"abi,opt[x86,armeabi-v7a,arm64-v8a,x86_64]"`
	ShouldUpdateEmulator bool   `env:"update,opt[yes,no]"`
	EmulatorChannel      string `env:"emulator_channel,opt[0,1,2,3]"`
	IsHeadlessMode       bool   `env:"headless_mode,opt[yes,no]"`
}

var (
	faultIndicators = []string{" BUG: ", "Kernel panic"}
)

const (
	bootTimeout         = time.Duration(10) * time.Minute
	deviceCheckInterval = time.Duration(5) * time.Second
	maxBootAttempts     = 5
)

func runningDeviceInfos(androidHome string) (map[string]string, error) {
	cmd := command.New(filepath.Join(androidHome, "platform-tools", "adb"), "devices")
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		log.Printf(err.Error())
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

	cpuIsARM, err := system.CPU.IsARM()
	if err != nil {
		log.Errorf("Failed to check CPU: %s", err)
	} else if cpuIsARM {
		log.Warnf("This Step is not yet supported on Apple Silicon (M1) machines. If you cannot find a solution to this error, try running this Workflow on an Intel-based machine type.")
	}

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
	name    string
	command *command.Model
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

	var phases []phase
	if cfg.ShouldUpdateEmulator {
		phases = []phase{
			{
				"Updating emulator",
				command.New(sdkManagerPath, "--verbose", "--channel="+cfg.EmulatorChannel, "emulator").
					SetStdin(strings.NewReader(yes)), // hitting yes in case it waits for accepting license
			},
			{
				"Updating system-image packages",
				command.New(sdkManagerPath, "--verbose", "--channel="+cfg.EmulatorChannel, pkg).
					SetStdin(strings.NewReader(yes)), // hitting yes in case it waits for accepting license
			},
		}
	}

	phases = append(phases, phase{
		"Creating device",
		command.New(avdManagerPath, append([]string{
			"--verbose", "create", "avd", "--force",
			"--name", cfg.ID,
			"--device", cfg.DeviceProfile,
			"--package", pkg,
			"--tag", cfg.Tag,
			"--abi", cfg.Abi}, createCustomFlags...)...).
			SetStdin(strings.NewReader(no)), // hitting no in case it asks for creating hw profile
	})

	for _, phase := range phases {
		log.Infof(phase.name)
		log.Donef("$ %s", phase.command.PrintableCommandArgs())

		if out, err := phase.command.RunAndReturnTrimmedCombinedOutput(); err != nil {
			failf("Failed to run phase: %s, output: %s", err, out)
		}

		fmt.Println()
	}

	args := []string{
		"@" + cfg.ID,
		"-verbose",
		"-show-kernel",
		"-no-audio",
		"-netdelay", "none",
		"-no-snapshot",
		"-wipe-data",
	}
	if !sliceutil.IsStringInSlice(startCustomFlags, "-gpu") {
		args = append(args, []string{"-gpu", "auto"}...)
	}
	if cfg.IsHeadlessMode {
		args = append(args, []string{"-no-window", "-no-boot-anim"}...)
	}
	args = append(args, startCustomFlags...)

	serial := startEmulator(emulatorPath, args, androidHome, runningDevices, 1)

	if err := tools.ExportEnvironmentWithEnvman("BITRISE_EMULATOR_SERIAL", serial); err != nil {
		log.Warnf("Failed to export environment (BITRISE_EMULATOR_SERIAL), error: %s", err)
	}
	log.Printf("- Device with serial: %s started", serial)

	log.Donef("- Done")
}

func startEmulator(emulatorPath string, args []string, androidHome string, runningDevices map[string]string, attempt int) string {
	var output bytes.Buffer
	deviceStartCmd := command.New(emulatorPath, args...).SetStdout(&output).SetStderr(&output)

	log.Infof("Starting device")
	log.Donef("$ %s", deviceStartCmd.PrintableCommandArgs())

	// The emulator command won't exit after the boot completes, so we start the command and not wait for its result.
	// Instead, we have a loop with 3 channels:
	// 1. One that waits for the emulator process to exit
	// 2. A boot timeout timer
	// 3. A ticker that periodically checks if the device has become online
	if err := deviceStartCmd.GetCmd().Start(); err != nil {
		failf("Failed to run device start command: %v", err)
	}

	emulatorWaitCh := make(chan error, 1)
	go func() {
		emulatorWaitCh <- deviceStartCmd.GetCmd().Wait()
	}()

	timeoutTimer := time.NewTimer(bootTimeout)

	deviceCheckTicker := time.NewTicker(deviceCheckInterval)

	var serial string
	retry := false
waitLoop:
	for {
		select {
		case err := <-emulatorWaitCh:
			log.Warnf("Emulator process exited early")
			if err != nil {
				log.Errorf("Emulator exit reason: %v", err)
			} else {
				log.Warnf("A possible cause can be the emulator process having received a KILL signal.")
			}
			log.Printf("Emulator log: %s", output)
			failf("Emulator exited early, see logs above.")
		case <-timeoutTimer.C:
			// Include error before and after printing the emulator log because it's so long
			errorMsg := fmt.Sprintf("Failed to boot emulator device within %d seconds.", bootTimeout/time.Second)
			log.Errorf(errorMsg)
			log.Printf("Emulator log: %s", output)
			failf(errorMsg)
		case <-deviceCheckTicker.C:
			var err error
			serial, err = queryNewDeviceSerial(androidHome, runningDevices)
			if err != nil {
				failf("Error: %s", err)
			} else if serial != "" {
				break waitLoop
			}
			if containsAny(output.String(), faultIndicators) {
				log.Warnf("Emulator log contains fault")
				log.Warnf("Emulator log: %s", output)
				if err := deviceStartCmd.GetCmd().Process.Kill(); err != nil {
					failf("Couldn't finish emulator process: %v", err)
				}
				if attempt < maxBootAttempts {
					log.Warnf("Trying to start emulator process again...")
					retry = true
					break waitLoop
				} else {
					failf("Failed to boot device due to faults after %d tries", maxBootAttempts)
				}
			}
		}
	}
	timeoutTimer.Stop()
	deviceCheckTicker.Stop()
	if retry {
		return startEmulator(emulatorPath, args, androidHome, runningDevices, attempt+1)
	}
	return serial
}

func containsAny(output string, any []string) bool {
	for _, fault := range any {
		if strings.Contains(output, fault) {
			return true
		}
	}

	return false
}
