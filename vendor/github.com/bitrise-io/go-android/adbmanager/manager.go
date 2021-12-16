package adbmanager

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bitrise-io/go-android/sdk"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/log"
)

// Manager ...
type Manager struct {
	sdk            sdk.AndroidSdkInterface
	commandFactory command.Factory
	logger         log.Logger
}

// NewManager ...
func NewManager(sdk sdk.AndroidSdkInterface, commandFactory command.Factory, logger log.Logger) Manager {
	return Manager{
		sdk:            sdk,
		commandFactory: commandFactory,
		logger:         logger,
	}
}

// StartServer ...
func (m Manager) StartServer() error {
	cmd := m.commandFactory.Create(m.adb(), []string{"start-server"}, &command.Opts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	m.logger.TDonef("$ %s", cmd.PrintableCommandArgs())
	return cmd.Run()
}

// KillServer ...
func (m Manager) KillServer() error {
	cmd := m.commandFactory.Create(m.adb(), []string{"kill-server"}, &command.Opts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	m.logger.TDonef("$ %s", cmd.PrintableCommandArgs())
	return cmd.Run()
}

// RestartServer ...
func (m Manager) RestartServer() error {
	if err := m.KillServer(); err != nil {
		return err
	}
	return m.StartServer()
}

// Devices ...
func (m Manager) Devices() (map[string]string, error) {
	cmd := m.commandFactory.Create(m.adb(), []string{"devices"}, nil)
	m.logger.TDonef("$ %s", cmd.PrintableCommandArgs())
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	m.logger.TPrintf(out)
	if err != nil {
		return nil, err
	}

	return parseDevicesOut(out)
}

// NewDevice ...
func (m Manager) NewDevice(previousDevices map[string]string) (string, string, error) {
	devices, err := m.Devices()
	if err != nil {
		return "", "", err
	}
	serial, state := findNewDevice(previousDevices, devices)
	return serial, state, nil
}

// WaitForDeviceShell ...
func (m Manager) WaitForDeviceShell(serial string, commands ...string) (string, error) {
	var args []string
	if serial != "" {
		args = append(args, "-s", serial)
	}
	args = append(args, "wait-for-device", "shell")
	args = append(args, commands...)

	cmd := m.commandFactory.Create(m.adb(), args, nil)
	m.logger.TDonef("$ %s", cmd.PrintableCommandArgs())
	return cmd.RunAndReturnTrimmedCombinedOutput()
}

// UnlockDevice ...
func (m Manager) UnlockDevice(serial string) (string, error) {
	return m.WaitForDeviceShell(serial, "input", "keyevent", "82")
}

// KillEmulator ...
func (m Manager) KillEmulator(serial string) error {
	cmd := m.commandFactory.Create(m.adb(), []string{"-s", serial, "emu", "kill"}, &command.Opts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	m.logger.TDonef("$ %s", cmd.PrintableCommandArgs())
	return cmd.Run()
}

func (m Manager) adb() string {
	return filepath.Join(m.sdk.PlatformTools(), "adb")
}

func parseDevicesOut(out string) (map[string]string, error) {
	// List of devices attached
	// emulator-5554	device
	const deviceListItemPattern = `^(?P<emulator>emulator-\d*)[\s+](?P<state>.*)`
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
		return nil, scanner.Err()
	}

	return deviceStateMap, nil
}

func findNewDevice(old, new map[string]string) (serial string, state string) {
	for serial, state = range new {
		_, ok := old[serial]
		if !ok {
			return
		}
	}
	return "", ""
}
