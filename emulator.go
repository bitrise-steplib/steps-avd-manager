package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitrise-io/go-android/adbmanager"
	"github.com/bitrise-io/go-android/sdk"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/log"
	asyncCmd "github.com/go-cmd/cmd"
)

type EmulatorManager struct {
	sdk        sdk.AndroidSdkInterface
	adbManager adbmanager.Manager
	logger     log.Logger
}

func NewEmulatorManager(sdk sdk.AndroidSdkInterface, commandFactory command.Factory, logger log.Logger) EmulatorManager {
	return EmulatorManager{
		sdk:        sdk,
		adbManager: adbmanager.NewManager(sdk, commandFactory, logger),
		logger:     logger,
	}
}

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
		m.logger.Warnf("failed to start adb server: %s", err)
		m.logger.Warnf("restarting adb server...")
		if err := m.adbManager.RestartServer(); err != nil {
			return "", fmt.Errorf("failed to restart adb server: %s", err)
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
			m.logger.Warnf("failed to terminate emulator: %s", err)
		}
		m.logger.Warnf("restarting emulator...")
		return m.StartEmulator(name, args, timeout)
	case serial := <-serialChan:
		return serial, nil
	case <-timeoutChan:
		return "", fmt.Errorf("timeout")
	}
}

func (m EmulatorManager) emulator() string {
	return filepath.Join(m.sdk.AndroidHome(), "emulator", "emulator")
}

func (m EmulatorManager) checkDeviceSerial(runningDevices map[string]string) chan string {
	serialChan := make(chan string)

	go func() {
		attempt := 0

		for {
			attempt++

			if attempt%10 == 0 {
				m.logger.Warnf("restarting adb server...")
				if err := m.adbManager.RestartServer(); err != nil {
					m.logger.Warnf("failed to restart adb server: %s", err)
				}
			}

			serial, state, err := m.adbManager.NewDevice(runningDevices)
			switch {
			case err != nil:
				m.logger.Warnf("failed to query new emulator: %s", err)
				m.logger.Warnf("restart adb server and retry")
				if err := m.adbManager.RestartServer(); err != nil {
					m.logger.Warnf("failed to restart adb server: %s", err)
				}

				attempt = 0 // avoid restarting adb server twice
			case serial != "":
				m.logger.Warnf("new emulator found: %s, state: %s", serial, state)
				if state == "device" {
					serialChan <- serial
					return
				}
			default:
				m.logger.Warnf("new emulator not found")
			}

			time.Sleep(2 * time.Second)
		}
	}()

	return serialChan
}

func (m EmulatorManager) handleOutput(stdoutChan, stderrChan <-chan string, errChan chan<- error) {
	handle := func(line string) {
		if containsAny(line, faultIndicators) {
			m.logger.Warnf("emulator log contains fault: %s", line)
			errChan <- fmt.Errorf("emulator start failed: %s", line)
			return
		}

		if strings.Contains(line, "INFO    | boot completed") {
			m.logger.Warnf("emulator log contains boot completed")
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
	}()
	return
}
