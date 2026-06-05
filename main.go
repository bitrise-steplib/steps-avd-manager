package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bitrise-io/go-android/v2/adbmanager"
	"github.com/bitrise-io/go-android/v2/sdk"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
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
	AndroidHome                string `env:"ANDROID_HOME"`
	DeployDir                  string `env:"BITRISE_DEPLOY_DIR"`
	APILevel                   string `env:"api_level,required"`
	Tag                        string `env:"tag,opt[google_apis,google_apis_ps16k,google_apis_playstore,google_apis_playstore_ps16k,aosp_atd,google_atd,android-wear,android-tv,default]"`
	DeviceProfile              string `env:"profile,required"`
	DisableAnimations          bool   `env:"disable_animations,opt[yes,no]"`
	CreateCommandArgs          string `env:"create_command_flags"`
	StartCommandArgs           string `env:"start_command_flags"`
	ID                         string `env:"emulator_id,required"`
	Abi                        string `env:"abi,opt[x86,armeabi-v7a,arm64-v8a,x86_64]"`
	EmulatorChannel            string `env:"emulator_channel,opt[no update,0,1,2,3]"`
	EmulatorBuildNumber        string `env:"emulator_build_number,required"`
	IsHeadlessMode             bool   `env:"headless_mode,opt[yes,no]"`
	HostDebugTags                  string `env:"host_debug_tags"`
	DeviceLogcatTags                 string `env:"device_logcat_tags"`
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
	hostLogSuffix              = "_host.log"
	deviceLogcatSuffix         = "_device_logcat.log"
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

		pkg     = fmt.Sprintf("system-images;android-%s;%s;%s", cfg.APILevel, cfg.Tag, cfg.Abi)
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

	createAVDArgs := []string{
		"--verbose", "create", "avd", "--force",
		"--name", cfg.ID,
		"--device", cfg.DeviceProfile,
		"--package", pkg,
		"--abi", cfg.Abi,
	}
	// ps16k images have a single valid avdmanager tag that varies by API level — let avdmanager auto-select it.
	// For all other tags, pass explicitly.
	if cfg.Tag != "google_apis_ps16k" && cfg.Tag != "google_apis_playstore_ps16k" {
		createAVDArgs = append(createAVDArgs, "--tag", cfg.Tag)
	}
	createAVDArgs = append(createAVDArgs, createCustomFlags...)

	phases = append(phases, []phase{
		{
			"Installing system image package",
			command.New(sdkManagerPath, "--verbose", "--channel="+systemImageChannel, pkg).
				SetStdin(strings.NewReader(yes)), // hitting yes in case it waits for accepting license
		},
		{
			"Creating device",
			command.New(avdManagerPath, createAVDArgs...).
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
		"-show-kernel",
		"-no-audio",
		"-netdelay", "none",
		"-no-snapshot",
		"-wipe-data",
	}
	if !slices.Contains(startCustomFlags, "-gpu") {
		args = append(args, []string{"-gpu", "auto"}...)
	}
	if cfg.IsHeadlessMode {
		args = append(args, []string{"-no-window", "-no-boot-anim"}...)
	}
	debugEnabled := cfg.HostDebugTags != "" && cfg.HostDebugTags != "none"
	logcatEnabled := cfg.DeviceLogcatTags != "" && cfg.DeviceLogcatTags != "none"

	// Detect debug/logcat flags already present in start_command_flags to avoid conflicts.
	customHasDebug := sliceutil.IsStringInSlice("-debug", startCustomFlags) ||
		sliceutil.IsStringInSlice("-verbose", startCustomFlags)
	customHasLogcat := sliceutil.IsStringInSlice("-logcat", startCustomFlags) ||
		sliceutil.IsStringInSlice("-logcat-output", startCustomFlags)

	if customHasDebug {
		failf("Conflicting flags: -debug or -verbose is already set in start_command_flags. Use the host_debug_tags input instead.")
	}
	if customHasLogcat && logcatEnabled {
		failf("Conflicting flags: -logcat/-logcat-output is already set in start_command_flags and device_logcat_tags is also set. Use one or the other.")
	}

	// Always pass -debug; use the user-specified tags or the default when host_debug_tags is not set.
	debugTags := "init,avd,kernel,snapshot"
	if debugEnabled {
		debugTags = cfg.HostDebugTags
	}
	args = append(args, "-debug", debugTags)

	// Timestamp embedded in filenames ensures uniqueness across retries and concurrent runs.
	runID := time.Now().Format("20060102_150405")

	var (
		emulatorLogPath string
		logcatLogPath   string
	)
	if cfg.DeployDir != "" {
		emulatorLogPath = filepath.Join(cfg.DeployDir, cfg.ID+"_"+runID+hostLogSuffix)
	}

	// Capture logcat for failure diagnostics unless the user already handles it via start_command_flags.
	if !customHasLogcat && cfg.DeployDir != "" {
		logcatTags := "*:w"
		if logcatEnabled {
			logcatTags = cfg.DeviceLogcatTags
		}
		logcatLogPath = filepath.Join(cfg.DeployDir, cfg.ID+"_"+runID+deviceLogcatSuffix)
		args = append(args, "-logcat", logcatTags, "-logcat-output", logcatLogPath)
	}

	args = append(args, startCustomFlags...)

	serial, bootErr := startEmulator(adbClient, emulatorPath, args, runningDevicesBeforeBoot, emulatorLogPath, 1)

	// On success, delete logs that weren't explicitly requested (they were captured for diagnostics only).
	if bootErr == nil {
		if emulatorLogPath != "" && !debugEnabled {
			if err := os.Remove(emulatorLogPath); err != nil {
				log.Warnf("Failed to remove emulator host log: %s", err)
			}
			emulatorLogPath = ""
		}
		if logcatLogPath != "" && !logcatEnabled {
			if err := os.Remove(logcatLogPath); err != nil {
				log.Warnf("Failed to remove device logcat log: %s", err)
			}
			logcatLogPath = ""
		}
	}

	if bootErr == nil && cfg.DisableAnimations {
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

	if serial != "" {
		if err := tools.ExportEnvironmentWithEnvman("BITRISE_EMULATOR_SERIAL", serial); err != nil {
			log.Warnf("Failed to export BITRISE_EMULATOR_SERIAL: %s", err)
		}
	}
	if emulatorLogPath != "" {
		if err := tools.ExportEnvironmentWithEnvman("BITRISE_EMULATOR_HOST_LOG", emulatorLogPath); err != nil {
			log.Warnf("Failed to export BITRISE_EMULATOR_HOST_LOG: %s", err)
		}
	}
	if logcatLogPath != "" {
		if err := tools.ExportEnvironmentWithEnvman("BITRISE_EMULATOR_DEVICE_LOGCAT_LOG", logcatLogPath); err != nil {
			log.Warnf("Failed to export BITRISE_EMULATOR_DEVICE_LOGCAT_LOG: %s", err)
		}
	}
	log.Printf("")
	log.Infof("Step outputs")
	if serial != "" {
		log.Printf("$BITRISE_EMULATOR_SERIAL = %s", serial)
	}
	if emulatorLogPath != "" {
		log.Printf("$BITRISE_EMULATOR_HOST_LOG = %s", emulatorLogPath)
	}
	if logcatLogPath != "" {
		log.Printf("$BITRISE_EMULATOR_DEVICE_LOGCAT_LOG = %s", logcatLogPath)
	}

	if bootErr != nil {
		failf(bootErr.Error())
	}
}

func startEmulator(adbClient adb.ADB, emulatorPath string, args []string, runningDevices map[string]string, logPath string, attempt int) (string, error) {
	var faultBuf bytes.Buffer
	var writer io.Writer = &faultBuf

	if logPath != "" {
		f, err := os.Create(logPath)
		if err != nil {
			log.Warnf("Failed to create emulator log file %s: %s", logPath, err)
		} else {
			defer func() {
				if err := f.Close(); err != nil {
					log.Warnf("Failed to close emulator log file: %s", err)
				}
			}()
			writer = io.MultiWriter(f, &faultBuf)
		}
	}

	deviceStartCmd := command.New(emulatorPath, args...).SetStdout(writer).SetStderr(writer)

	log.Infof("Starting device")
	log.Donef("$ %s", deviceStartCmd.PrintableCommandArgs())

	// The emulator command won't exit after the boot completes, so we start the command and not wait for its result.
	// Instead, we have a loop with 3 channels:
	// 1. One that waits for the emulator process to exit
	// 2. A boot timeout timer
	// 3. A ticker that periodically checks if the device has become online
	if err := deviceStartCmd.GetCmd().Start(); err != nil {
		return "", fmt.Errorf("failed to run device start command: %v", err)
	}

	emulatorWaitCh := make(chan error, 1)
	go func() {
		emulatorWaitCh <- deviceStartCmd.GetCmd().Wait()
	}()

	timeoutTimer := time.NewTimer(bootTimeout)

	deviceCheckTicker := time.NewTicker(deviceCheckInterval)

	printLogHint := func() {
		log.Printf("Emulator log tail:\n%s", tailLines(faultBuf.String(), 50))
		if logPath != "" {
			log.Printf("Full emulator log: %s", logPath)
		}
	}

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
			printLogHint()
			return "", fmt.Errorf("emulator exited early, see logs above")
		case <-timeoutTimer.C:
			log.Errorf("Failed to boot emulator device within %d seconds.", bootTimeout/time.Second)
			printLogHint()
			return "", fmt.Errorf("failed to boot emulator device within %d seconds", bootTimeout/time.Second)
		case <-deviceCheckTicker.C:
			var err error
			serial, err = adbClient.FindNewDevice(runningDevices)
			if err != nil {
				return "", fmt.Errorf("finding new device: %s", err)
			} else if serial != "" {
				break waitLoop
			}
			if containsAny(faultBuf.String(), faultIndicators) {
				log.Warnf("Emulator log contains fault")
				printLogHint()
				if err := deviceStartCmd.GetCmd().Process.Kill(); err != nil {
					return "", fmt.Errorf("couldn't finish emulator process: %v", err)
				}
				if attempt < maxBootAttempts {
					log.Warnf("Trying to start emulator process again...")
					retry = true
					break waitLoop
				} else {
					return "", fmt.Errorf("failed to boot device due to faults after %d tries", maxBootAttempts)
				}
			}
		}
	}
	timeoutTimer.Stop()
	deviceCheckTicker.Stop()
	if retry {
		return startEmulator(adbClient, emulatorPath, args, runningDevices, logPath, attempt+1)
	}
	return serial, nil
}

func tailLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func containsAny(output string, any []string) bool {
	for _, fault := range any {
		if strings.Contains(output, fault) {
			return true
		}
	}

	return false
}
