package adbmanager

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/bitrise-io/go-android/v2/sdk"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/log"
)

type Model struct {
	binPth     string
	cmdFactory command.Factory
	logger     log.Logger
}

func New(sdk sdk.AndroidSdkInterface, cmdFactory command.Factory, logger log.Logger) (*Model, error) {
	binPth := filepath.Join(sdk.GetAndroidHome(), "platform-tools", "adb")
	if exist, err := pathutil.IsPathExists(binPth); err != nil {
		return nil, fmt.Errorf("failed to check if adb exist, error: %s", err)
	} else if !exist {
		return nil, fmt.Errorf("adb not exist at: %s", binPth)
	}

	return &Model{
		binPth:     binPth,
		cmdFactory: cmdFactory,
		logger:     logger,
	}, nil
}

func (model Model) DevicesCmd() *command.Command {
	cmd := model.cmdFactory.Create(model.binPth, []string{"devices"}, nil)
	return &cmd
}

func (model Model) UnlockDevice(serial string) error {
	keyEvent82Cmd := model.cmdFactory.Create(model.binPth, []string{"-s", serial, "shell", "input", "82", "&"}, nil)
	if err := keyEvent82Cmd.Run(); err != nil {
		return err
	}

	keyEvent1Cmd := model.cmdFactory.Create(model.binPth, []string{"-s", serial, "shell", "input", "1", "&"}, nil)
	return keyEvent1Cmd.Run()
}

// InstallAPKCmd builds and returns a `Command` for installing APKs on an attached device or emulator.
// The `Command` can than be run by the consumer without needing to know the implementation details.
func (model Model) InstallAPKCmd(pathToAPK string, commandOptions *command.Opts) command.Command {
	cmd := model.cmdFactory.Create(model.binPth, []string{"install", pathToAPK}, commandOptions)
	return cmd
}

// RunInstrumentedTestsCmd builds and returns a `Command` for running instrumented tests on an attached device or emulator.
// The `Command` can than be run by the consumer without needing to know the implementation details.
//
// `additionalTestingOptions` is a list of arbitrary key value pairs to be passed to the test runner.
//
// Example:
//
// If a value of `{"KEY1", "value1", "KEY2", "value2"}` is passed to `additionalTestingOptions`,
// then it will be passed to the `adb` command like so:
//
// adb shell am instrument -e "KEY1" "value1" "KEY2" "value2" [...]
//
// See `adb` documentation for more info: https://developer.android.com/studio/command-line/adb#am
func (model Model) RunInstrumentedTestsCmd(
	packageName string,
	testRunnerClass string,
	additionalTestingOptions []string,
	commandOptions *command.Opts,
) command.Command {
	args := []string{
		"shell",
		"am",
		"instrument",
		"-w", // Tells `am` (activity manager) to wait for instrumentation to finish before returning
	}

	if len(additionalTestingOptions) > 0 {
		args = append(args, "-e")
		args = append(args, additionalTestingOptions...)
	}

	component := packageName + "/" + testRunnerClass
	args = append(args, component)

	cmd := model.cmdFactory.Create(model.binPth, args, commandOptions)
	return cmd
}

func (model Model) WaitForDevice(serial string, timeout time.Duration) error {
	startTime := time.Now()

	for {
		model.logger.Printf("Waiting for emulator to boot...")

		bootCompleteChan := model.getBootCompleteEvent(serial, timeout)
		result := <-bootCompleteChan
		switch {
		case result.Error != nil:
			model.logger.Warnf("Failed to check emulator boot status: %s", result.Error)
			model.logger.Warnf("Killing ADB server before retry...")
			killCmd := model.KillServerCmd(nil)
			if out, err := killCmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
				return fmt.Errorf("terminate adb server: %s", out)
			}
		case result.Booted:
			model.logger.Donef("Device boot completed in %d seconds", time.Since(startTime)/time.Second)
			return nil
		}

		if time.Now().After(startTime.Add(timeout)) {
			return fmt.Errorf("emulator boot check timed out after %s seconds", time.Since(startTime)/time.Second)
		}

		delay := 5 * time.Second
		model.logger.Printf("Device is online but still booting, retrying in %d seconds", delay/time.Second)
		time.Sleep(delay)
	}
}

// WaitForDeviceThenShellCmd returns a command that first waits for a device to come online, then executes the provided
// command(s) on the device shell
func (model Model) WaitForDeviceThenShellCmd(serial string, commandOptions *command.Opts, commands ...string) command.Command {
	var args []string
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "wait-for-device", "shell")
	args = append(args, commands...)

	cmd := model.cmdFactory.Create(model.binPth, args, commandOptions)
	return cmd
}

// KillServerCmd returns a command that kills the ADB server if it is running.
// The next ADB command will automatically start the server.
func (model Model) KillServerCmd(commandOptions *command.Opts) command.Command {
	cmd := model.cmdFactory.Create(model.binPth, []string{"kill-server"}, commandOptions)
	return cmd
}
