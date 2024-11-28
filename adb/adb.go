package adb

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/log"
)

type ADB struct {
	androidHome string
	cmdFactory  command.Factory
	logger      log.Logger
}

func New(androidHome string, cmdFactory command.Factory, logger log.Logger) ADB {
	return ADB{
		androidHome: androidHome,
		cmdFactory:  cmdFactory,
		logger:      logger,
	}
}

const DeviceStateConnected = "device"

// Key: device serial number
// Value: device state
type Devices map[string]string

// Devices returns a map of connected Android devices and their states.
func (a *ADB) Devices() (Devices, error) {
	cmd := a.cmdFactory.Create(
		filepath.Join(a.androidHome, "platform-tools", "adb"),
		[]string{"devices"},
		nil,
	)
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		a.logger.Printf(out)
		return map[string]string{}, fmt.Errorf("adb devices: %s", err)
	}

	a.logger.Debugf("$ %s", cmd.PrintableCommandArgs())
	a.logger.Debugf("%s", out)

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
		return map[string]string{}, fmt.Errorf("scan adb devices output: %s", err)
	}

	return deviceStateMap, nil
}

// FindNewDevice returns the serial number of a newly connected device compared
// to the previous state of running devices.
// If no new device is found, an empty string is returned.
func (a *ADB) FindNewDevice(previousDeviceState Devices) (string, error) {
	devicesNow, err := a.Devices()
	if err != nil {
		return "", err
	}

	newDeviceSerial := ""
	for serial := range devicesNow {
		_, found := previousDeviceState[serial]
		if !found {
			newDeviceSerial = serial
			break
		}
	}

	if len(newDeviceSerial) > 0 {
		state := devicesNow[newDeviceSerial]
		if state == DeviceStateConnected {
			return newDeviceSerial, nil
		}
	}

	return "", nil
}
