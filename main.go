package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-android/v2/adbmanager"
	"github.com/bitrise-io/go-android/v2/sdk"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-io/go-utils/sliceutil"
	v2command "github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	v2log "github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-io/go-utils/v2/retryhttp"
	"github.com/bitrise-io/go-utils/v2/system"
	"github.com/bitrise-steplib/steps-avd-manager/adb"
	"github.com/bitrise-steplib/steps-avd-manager/emuinstaller"
	"github.com/kballard/go-shellquote"
)

type config struct {
	AndroidHome         string `env:"ANDROID_HOME"`
	APILevel            int    `env:"api_level,required"`
	Tag                 string `env:"tag,opt[google_apis,google_apis_playstore,aosp_atd,google_atd,android-wear,android-tv,default]"`
	DeviceProfile       string `env:"profile,required"`
	DisableAnimations   bool   `env:"disable_animations,opt[yes,no]"`
	CreateCommandArgs   string `env:"create_command_flags"`
	StartCommandArgs    string `env:"start_command_flags"`
	ID                  string `env:"emulator_id,required"`
	Abi                 string `env:"abi,opt[x86,armeabi-v7a,arm64-v8a,x86_64]"`
	EmulatorChannel     string `env:"emulator_channel,opt[no update,0,1,2,3]"`
	EmulatorBuildNumber string `env:"emulator_build_number,required"`
	IsHeadlessMode      bool   `env:"headless_mode,opt[yes,no]"`
}

var (
	faultIndicators = []string{" BUG: ", "Kernel panic"}
)

const (
	bootTimeout                = time.Duration(10) * time.Minute
	deviceCheckInterval        = time.Duration(5) * time.Second
	maxBootAttempts            = 5
	emuChannelNoUpdate         = "no update"
	emuBuildNumberPreinstalled = "preinstalled"
)

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

type phase struct {
	name    string
	command *command.Model
}

func validateConfig(cfg config) error {
	if cfg.EmulatorChannel != emuChannelNoUpdate && cfg.EmulatorBuildNumber != emuBuildNumberPreinstalled {
		return fmt.Errorf("emulator_channel is set to `%s`, and emulator_build_number is also set to `%s`. These inputs are exclusive, please set either of them to the default value", cfg.EmulatorChannel, cfg.EmulatorBuildNumber)
	}

	return nil
}

func main() {
	cmdFactory := v2command.NewFactory(env.NewRepository())
	logger := v2log.NewLogger()

	var cfg config
	if err := stepconf.Parse(&cfg); err != nil {
		failf("Couldn't parse step inputs: %s", err)
	}
	stepconf.Print(cfg)
	fmt.Println()

	if err := validateConfig(cfg); err != nil {
		failf("Step input validation failed: %s", err)
	}

	// Initialize Android SDK
	log.Infof("Initialize Android SDK")
	androidSdk, err := sdk.New(cfg.AndroidHome)
	if err != nil {
		failf("Failed to initialize Android SDK: %s", err)
	}

	adbClient := adb.New(cfg.AndroidHome, cmdFactory, logger)
	runningDevicesBeforeBoot, err := adbClient.Devices()
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
		emulatorPath   = filepath.Join(cfg.AndroidHome, "emulator", "emulator")

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

	if cfg.EmulatorBuildNumber != emuBuildNumberPreinstalled {
		httpClient := retryhttp.NewClient(logger)
		emuInstaller := emuinstaller.NewEmuInstaller(cfg.AndroidHome, cmdFactory, logger, httpClient)
		if err := emuInstaller.Install(cfg.EmulatorBuildNumber); err != nil {
			failf("Failed to install emulator build %s: %s", cfg.EmulatorBuildNumber, err)
		}
	}

	var (
		systemImageChannel = "0"
		phases             []phase
	)
	if cfg.EmulatorChannel != emuChannelNoUpdate {
		systemImageChannel = cfg.EmulatorChannel
		phases = append(phases,
			phase{
				"Updating emulator",
				command.New(sdkManagerPath, "--verbose", "--channel="+cfg.EmulatorChannel, "emulator").
					SetStdin(strings.NewReader(yes)), // hitting yes in case it waits for accepting license
			},
		)
	}

	phases = append(phases, []phase{
		{
			"Installing system image package",
			command.New(sdkManagerPath, "--verbose", "--channel="+systemImageChannel, pkg).
				SetStdin(strings.NewReader(yes)), // hitting yes in case it waits for accepting license
		},
		{
			"Creating device",
			command.New(avdManagerPath, append([]string{
				"--verbose", "create", "avd", "--force",
				"--name", cfg.ID,
				"--device", cfg.DeviceProfile,
				"--package", pkg,
				"--tag", cfg.Tag,
				"--abi", cfg.Abi}, createCustomFlags...)...).
				SetStdin(strings.NewReader(no)), // hitting no in case it asks for creating hw profile
		},
	}...)

	for _, phase := range phases {
		log.Infof(phase.name)
		log.Donef("$ %s", phase.command.PrintableCommandArgs())

		startTime := time.Now()
		if out, err := phase.command.RunAndReturnTrimmedCombinedOutput(); err != nil {
			log.Printf("Duration: %s", time.Since(startTime))
			failf("Failed to run phase: %s, output: %s", err, out)
		}
		log.Printf("Duration: %s", time.Since(startTime).Round(time.Millisecond))

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
	if !sliceutil.IsStringInSlice("-gpu", startCustomFlags) {
		args = append(args, []string{"-gpu", "auto"}...)
	}
	if cfg.IsHeadlessMode {
		args = append(args, []string{"-no-window", "-no-boot-anim"}...)
	}
	args = append(args, startCustomFlags...)

	serial := startEmulator(adbClient, emulatorPath, args, cfg.AndroidHome, runningDevicesBeforeBoot, 1)

	if cfg.DisableAnimations {
		// We need to wait for the device to boot before we can disable animations
		adb, err := adbmanager.New(androidSdk, cmdFactory, logger)
		if err != nil {
			failf("Failed to create ADB model: %s", err)
		}
		err = adb.WaitForDevice(serial, bootTimeout)
		if err != nil {
			failf(err.Error())
		}

		err = adbClient.DisableAnimations(serial)
		if err != nil {
			failf("Failed to disable animations: %s", err)
		}
		log.Donef("Done")
	}

	if err := tools.ExportEnvironmentWithEnvman("BITRISE_EMULATOR_SERIAL", serial); err != nil {
		log.Warnf("Failed to export environment (BITRISE_EMULATOR_SERIAL), error: %s", err)
	}
	log.Printf("")
	log.Infof("Step outputs")
	log.Printf("$BITRISE_EMULATOR_SERIAL=%s", serial)
}

func startEmulator(adbClient adb.ADB, emulatorPath string, args []string, androidHome string, runningDevices map[string]string, attempt int) string {
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
			serial, err = adbClient.FindNewDevice(runningDevices)
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
		return startEmulator(adbClient, emulatorPath, args, androidHome, runningDevices, attempt+1)
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
