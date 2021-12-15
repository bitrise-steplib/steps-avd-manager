package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-android/sdk"
	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/log"
	commandv2 "github.com/bitrise-io/go-utils/v2/command"
	envv2 "github.com/bitrise-io/go-utils/v2/env"
	logv2 "github.com/bitrise-io/go-utils/v2/log"
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

var (
	faultIndicators = []string{" BUG: ", "Kernel panic"}
)

func failf(msg string, args ...interface{}) {
	log.Errorf(msg, args...)
	os.Exit(1)
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

	androidHome := androidSdk.AndroidHome()
	cmdlineToolsPath, err := androidSdk.CmdlineTools()
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
		{
			"Updating emulator",
			command.New(sdkManagerPath, "--verbose", "--channel="+cfg.EmulatorChannel, "emulator").
				SetStdin(strings.NewReader(yes)), // hitting yes in case it waits for accepting license
		},

		{
			"Updating system-image packages",
			command.New(sdkManagerPath, "--verbose", pkg).
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
	} {
		log.Infof(phase.name)
		log.TDonef("$ %s", phase.command.PrintableCommandArgs())

		if out, err := phase.command.RunAndReturnTrimmedCombinedOutput(); err != nil {
			failf("Failed to run phase: %s, output: %s", err, out)
		}

		fmt.Println()
	}

	printEmulatorVersion(emulatorPath)

	emulatorManager := NewEmulatorManager(androidSdk, commandv2.NewFactory(envv2.NewRepository()), logv2.NewLogger())

	fmt.Println()
	log.Infof("Start emulator")

	serial, err := emulatorManager.StartEmulator(cfg.ID, startCustomFlags, 10*time.Minute)
	if err != nil {
		failf(err.Error())
	}

	if err := tools.ExportEnvironmentWithEnvman("BITRISE_EMULATOR_SERIAL", serial); err != nil {
		log.Warnf("Failed to export environment (BITRISE_EMULATOR_SERIAL), error: %s", err)
	}
	log.Printf("- Device with serial: %s started", serial)

	log.Donef("- Done")
}

func printEmulatorVersion(emulatorPath string) {
	cmd := command.New(emulatorPath, "-version")

	log.Infof("Emulator version:")
	log.TDonef("$ %s", cmd.PrintableCommandArgs())

	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		log.Warnf("Failed to print emulator versions: %s", err)
	}
	s := strings.Split(out, "\n")
	if len(s) > 0 {
		log.Printf(s[0])
	} else {
		log.Printf(out)
	}
}

func containsAny(output string, any []string) bool {
	for _, fault := range any {
		if strings.Contains(output, fault) {
			return true
		}
	}

	return false
}
