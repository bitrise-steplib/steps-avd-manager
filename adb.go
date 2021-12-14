package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/log"
)

type ADBManager struct {
	androidHome    string
	commandFactory command.Factory
	logger         log.Logger
}

func NewADBManager(androidHome string, commandFactory command.Factory, logger log.Logger) ADBManager {
	return ADBManager{
		androidHome:    androidHome,
		commandFactory: commandFactory,
		logger:         logger,
	}
}

func (m ADBManager) adb() string {
	return filepath.Join(m.androidHome, "platform-tools", "adb")
}

func (m ADBManager) StartServer() error {
	cmd := m.commandFactory.Create(m.adb(), []string{"start-server"}, &command.Opts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	m.logger.TDonef("$ %s", cmd.PrintableCommandArgs())
	return cmd.Run()
}

func (m ADBManager) Devices() (map[string]string, error) {
	cmd := m.commandFactory.Create(m.adb(), []string{"devices"}, nil)
	m.logger.TDonef("$ %s", cmd.PrintableCommandArgs())
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	fmt.Println(out)
	if err != nil {
		return nil, err
	}

	return parseDevicesOut(out)
}

func (m ADBManager) FirstNewDeviceSerial(previousDevices map[string]string) (string, error) {
	devices, err := m.Devices()
	if err != nil {
		return "", err
	}
	return findFirstNewDeviceSerial(previousDevices, devices), nil

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
		return nil, fmt.Errorf("scanner failed, error: %s", scanner.Err())
	}

	return deviceStateMap, nil
}

func findFirstNewDeviceSerial(old, new map[string]string) string {
	for s := range new {
		_, ok := old[s]
		if !ok {
			return s
		}
	}
	return ""
}
