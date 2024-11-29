package adb

import (
	"testing"

	"github.com/bitrise-io/go-utils/v2/log"
	"github.com/bitrise-steplib/steps-avd-manager/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindNewDevice(t *testing.T) {
	androidHome := "/fake/android/home"
	logger := log.NewLogger()

	tests := []struct {
		name            string
		previousDevices Devices
		adbOutput       string
		expectedSerial  string
	}{
		{
			name: "no new device",
			previousDevices: Devices{
				"emulator-5554": "device",
			},
			adbOutput:      "List of devices attached\nemulator-5554\tdevice\n",
			expectedSerial: "",
		},
		{
			name: "new device connected",
			previousDevices: Devices{
				"emulator-5554": "device",
			},
			adbOutput:      "List of devices attached\nemulator-5554\tdevice\nemulator-5556\tdevice\n",
			expectedSerial: "emulator-5556",
		},
		{
			name: "new device not connected",
			previousDevices: Devices{
				"emulator-5554": "device",
			},
			adbOutput:      "List of devices attached\nemulator-5554\tdevice\nemulator-5556\toffline\n",
			expectedSerial: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdFactory := test.FakeCommandFactory{
				Stdout:  tt.adbOutput,
				ExitCode: 0,
			}
			adb := New(androidHome, cmdFactory, logger)

			newDevice, err := adb.FindNewDevice(tt.previousDevices)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedSerial, newDevice)
		})
	}
}
