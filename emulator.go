package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-android/adbmanager"
	"github.com/bitrise-io/go-android/sdk"
	androidSDK "github.com/bitrise-io/go-android/sdk"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/log"
	asyncCmd "github.com/go-cmd/cmd"
)

type EmulatorManager struct {
	sdk        sdk.AndroidSdkInterface
	adbManager adbmanager.Manager
	logger     log.Logger
}

func NewEmulatorManager(sdk androidSDK.AndroidSdkInterface, commandFactory command.Factory, logger log.Logger) EmulatorManager {
	return EmulatorManager{
		sdk:        sdk,
		adbManager: adbmanager.NewManager(sdk, commandFactory, logger),
		logger:     logger,
	}
}

// TODO: Your emulator is out of date, please update by launching Android Studio:
// https://app.bitrise.io/build/0b902ceb-c3fd-4c24-abf0-0768226433fb#?tab=log
func (m EmulatorManager) StartEmulator(name string, args []string, timeout time.Duration) (string, error) {
	args = append([]string{
		"@" + name,
		"-verbose",
		"-show-kernel",
		"-no-audio",
		"-no-window",
		"-no-boot-anim",
		"-netdelay", "none",
		"-no-snapshot",
		"-wipe-data",
		"-gpu", "swiftshader_indirect"}, args...)

	if err := m.adbManager.StartServer(); err != nil {
		if err := m.adbManager.RestartServer(); err != nil {
			failf("Failed to start adb server: %s", err)
		}
	}

	devices, err := m.adbManager.Devices()
	if err != nil {
		return "", err
	}

	timeoutChan := time.After(timeout)

	m.logger.TDonef("$ %s", strings.Join(append([]string{m.emulator()}, args...), " "))

	cmdOptions := asyncCmd.Options{Buffered: false, Streaming: true}
	cmd := asyncCmd.NewCmdOptions(cmdOptions, m.emulator(), args...)

	errChan := make(chan error)

	serialChan := m.checkDeviceSerial(devices)
	stdoutChan, stderrChan := m.broadcastStdoutAndStderr(cmd)
	go m.handleOutput(stdoutChan, stderrChan, errChan)

	select {
	case <-cmd.Start():
		m.logger.Warnf("emulator exited unexpectedly")
		return m.StartEmulator(name, args, timeout)
	case err := <-errChan:
		m.logger.Warnf("error occurred: %", err)
		if err := cmd.Stop(); err != nil {
			m.logger.Warnf("Failed to terminate emulator command: %s", err)
		}
		m.logger.Warnf("restarting emulator...")
		return m.StartEmulator(name, args, timeout)
	case serial := <-serialChan:
		return serial, nil
	case <-timeoutChan:
		m.logger.Warnf("timeout")
		return "", fmt.Errorf("timeout")
	}
}

func (m EmulatorManager) emulator() string {
	return filepath.Join(m.sdk.AndroidHome(), "emulator", "emulator")
}

func (m EmulatorManager) checkDeviceSerial(runningDevices map[string]string) chan string {
	serialChan := make(chan string)

	go func() {
		for {
			serial, state, err := m.adbManager.NewDevice(runningDevices)
			switch {
			case err != nil:
				m.logger.Warnf("failed to query serial: %s", err)
				m.logger.Warnf("restart adb server and retry...")
				if err := m.adbManager.RestartServer(); err != nil {
					m.logger.Warnf("failed to restart adb server: %s", err)
				}
			case serial != "":
				m.logger.Warnf("new emulator found: %s, state: %s", serial, state)
				if state == "device" {
					serialChan <- serial
					return
				}
			default:
				m.logger.Warnf("serial not found")
			}

			time.Sleep(2 * time.Second)
		}
	}()

	return serialChan
}

func (m EmulatorManager) handleOutput(stdoutChan, stderrChan <-chan string, errChan chan<- error) {
	handle := func(line string) {
		if containsAny(line, faultIndicators) {
			m.logger.Warnf("Emulator log contains fault: %s", line)
			errChan <- fmt.Errorf("emulator start failed: %s", line)
			return
		}

		if strings.Contains(line, "INFO    | boot completed") {
			m.logger.Warnf("It seems boot completed")
		}
	}

	for {
		select {
		case line := <-stdoutChan:
			fmt.Fprintln(os.Stdout, line)
			handle(line)
		case line := <-stderrChan:
			fmt.Fprintln(os.Stderr, line)
			handle(line)
		}
	}
}

func (m EmulatorManager) broadcastStdoutAndStderr(cmd *asyncCmd.Cmd) (stdoutChan chan string, stderrChan chan string) {
	stdoutChan, stderrChan = make(chan string), make(chan string)
	go func() {
		for cmd.Stdout != nil || cmd.Stderr != nil {
			select {
			case line, open := <-cmd.Stdout:
				if !open {
					cmd.Stdout = nil
					continue
				}

				stdoutChan <- line
			case line, open := <-cmd.Stderr:
				if !open {
					cmd.Stderr = nil
					continue
				}

				stderrChan <- line
			}
		}

		m.logger.Warnf("stdout and stderr is closed")
	}()
	return
}
