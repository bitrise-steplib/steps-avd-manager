package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	asyncCmd "github.com/go-cmd/cmd"

	"github.com/bitrise-io/go-utils/log"
)

func checkDeviceSerial(androidHome string, runningDevices map[string]string, errChan chan error, serialChan chan string) {
	var serial string

	for {
		var err error
		serial, err = queryNewDeviceSerial(androidHome, runningDevices)
		if err != nil {
			log.Warnf("failed to query serial: %s", err)
			errChan <- err
		} else if serial != "" {
			log.Warnf("serial found: %s", serial)
			serialChan <- serial
		}

		time.Sleep(5 * time.Second)
	}
}

func handleFault(line string, cmd *asyncCmd.Cmd) {
	if containsAny(line, faultIndicators) {
		log.Warnf("Emulator log contains fault")
		log.Warnf("Emulator log: %s", line)
		if err := cmd.Stop(); err != nil {
			log.Warnf("Failed to terminate command: %s", err)
		}
	}
}

func handleOutput(cmd *asyncCmd.Cmd, stdoutChan, stderrChan <-chan string, doneChan <-chan struct{}) {
	for {
		select {
		case line := <-stdoutChan:
			fmt.Fprintln(os.Stdout, line)
			handleFault(line, cmd)
		case line := <-stderrChan:
			fmt.Fprintln(os.Stderr, line)
			handleFault(line, cmd)
		case <-doneChan:
			return
		}
	}
}

func startEmulator2(emulatorPath string, args []string, androidHome string, runningDevices map[string]string, timeoutChan <-chan time.Time) (string, error) {
	log.TDonef("$ %s", strings.Join(append([]string{emulatorPath}, args...), " "))

	cmdOptions := asyncCmd.Options{Buffered: false, Streaming: true}
	cmd := asyncCmd.NewCmdOptions(cmdOptions, emulatorPath, args...)

	doneChan := make(chan struct{})
	stdoutChan := make(chan string)
	stderrChan := make(chan string)
	errChan := make(chan error)
	serialChan := make(chan string)

	go handleOutput(cmd, stdoutChan, stderrChan, doneChan)
	go checkDeviceSerial(androidHome, runningDevices, errChan, serialChan)
	go broadcastStdoutAndStderr(cmd, stdoutChan, stderrChan, doneChan)

	select {
	case <-cmd.Start():
		log.Warnf("emulator exited unexpectedly")
		return startEmulator2(emulatorPath, args, androidHome, runningDevices, timeoutChan)
	case err := <-errChan:
		log.Warnf("error occurred: %", err)
		return startEmulator2(emulatorPath, args, androidHome, runningDevices, timeoutChan)
	case serial := <-serialChan:
		return serial, nil
	case <-timeoutChan:
		log.Warnf("timeout")
		return "", fmt.Errorf("timeout")

	}
}
