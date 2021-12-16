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

var (
	faultIndicators = []string{" BUG: ", "Kernel panic"}
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

func (m EmulatorManager) StartEmulator(name string, args []string, timeoutChan <-chan time.Time) (string, error) {
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
		m.logger.TWarnf("failed to start adb server: %s", err)
		m.logger.TWarnf("restarting adb server...")
		if err := m.adbManager.RestartServer(); err != nil {
			return "", fmt.Errorf("failed to restart adb server: %s", err)
		}
	}

	devices, err := m.adbManager.Devices()
	if err != nil {
		return "", err
	}

	m.logger.TDonef("$ %s", strings.Join(append([]string{m.emulator()}, args...), " "))

	cmdOptions := asyncCmd.Options{Buffered: false, Streaming: true}
	cmd := asyncCmd.NewCmdOptions(cmdOptions, m.emulator(), args...)

	errChan := make(chan error)

	stdoutChan, stderrChan := m.broadcastStdoutAndStderr(cmd)
	go m.handleOutput(stdoutChan, stderrChan, errChan)

	serialChan := make(chan QueryNewDeviceResult)
	time.AfterFunc(1*time.Minute, func() {
		m.queryNewDevice(devices, serialChan)
	})

	serial := ""
	for {
		select {
		case <-cmd.Start():
			m.logger.TWarnf("emulator exited unexpectedly")
			return m.StartEmulator(name, args, timeoutChan)
		case err := <-errChan:
			m.logger.TWarnf("error occurred: %s", err)

			if err := cmd.Stop(); err != nil {
				m.logger.TWarnf("failed to terminate emulator: %s", err)
			}

			if serial != "" {
				if err := m.adbManager.KillEmulator(serial); err != nil {
					m.logger.TWarnf("failed to kill %s: %s", serial, err)
				}
			}

			m.logger.TWarnf("restarting emulator...")
			return m.StartEmulator(name, args, timeoutChan)
		case res := <-serialChan:
			serial = res.Serial
			if res.State == "device" {
				return res.Serial, nil
			}
		case <-timeoutChan:
			return "", fmt.Errorf("timeout")
		}
	}
}

func (m EmulatorManager) emulator() string {
	return filepath.Join(m.sdk.AndroidHome(), "emulator", "emulator")
}

type QueryNewDeviceResult struct {
	Serial string
	State  string
}

func (m EmulatorManager) queryNewDevice(runningDevices map[string]string, serialChan chan<- QueryNewDeviceResult) {
	const sleepTime = 5 * time.Second

	go func() {
		attempt := 0

		for {
			attempt++

			// Restart adb server as emulator does not show up in a device state quite a while ago.
			if attempt%10 == 0 {
				m.logger.TWarnf("restarting adb server...")
				if err := m.adbManager.RestartServer(); err != nil {
					m.logger.TWarnf("failed to restart adb server: %s", err)
				}
			}

			serial, state, err := m.adbManager.NewDevice(runningDevices)
			switch {
			case err != nil:
				m.logger.TWarnf("failed to query new emulator: %s", err)
				m.logger.TWarnf("restart adb server and retry")
				if err := m.adbManager.RestartServer(); err != nil {
					m.logger.TWarnf("failed to restart adb server: %s", err)
				}

				attempt = 0
			case serial != "":
				m.logger.TWarnf("new emulator found: %s, state: %s", serial, state)
				serialChan <- QueryNewDeviceResult{Serial: serial, State: state}
			default:
				m.logger.TWarnf("new emulator not found")
			}

			time.Sleep(sleepTime)
		}
	}()
}

func (m EmulatorManager) handleOutput(stdoutChan, stderrChan <-chan string, errChan chan<- error) {
	handle := func(line string) {
		if containsAny(line, faultIndicators) {
			m.logger.TWarnf("emulator log contains fault: %s", line)
			errChan <- fmt.Errorf("emulator start failed: %s", line)
			return
		}

		if strings.Contains(line, "INFO    | boot completed") {
			m.logger.TWarnf("emulator log contains boot completed")
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

func containsAny(output string, any []string) bool {
	for _, fault := range any {
		if strings.Contains(output, fault) {
			return true
		}
	}

	return false
}
