package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bitrise-io/go-utils/log"
	asyncCmd "github.com/go-cmd/cmd"
)

func checkDeviceSerial(androidHome string, runningDevices map[string]string, errChan chan<- error) chan string {
	serialChan := make(chan string)

	go func() {
		for {
			serial, err := queryNewDeviceSerial(androidHome, runningDevices)
			switch {
			case err != nil:
				log.Warnf("failed to query serial: %s", err)
				errChan <- err
				return
			case serial != "":
				log.Warnf("serial found: %s", serial)
				serialChan <- serial
				return
			default:
				log.Warnf("serial not found")
			}

			time.Sleep(2 * time.Second)
		}
	}()

	return serialChan
}

func handleOutput(stdoutChan, stderrChan <-chan string, errChan chan<- error) {
	handleFault := func(line string) {
		if containsAny(line, faultIndicators) {
			log.Warnf("Emulator log contains fault: %s", line)
			errChan <- fmt.Errorf("emulator start failed: %s", line)
		}
	}

	for {
		select {
		case line := <-stdoutChan:
			fmt.Fprintln(os.Stdout, line)
			handleFault(line)
		case line := <-stderrChan:
			fmt.Fprintln(os.Stderr, line)
			handleFault(line)
		}
	}
}

func broadcastStdoutAndStderr(cmd *asyncCmd.Cmd) (stdoutChan chan string, stderrChan chan string) {
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

		log.Warnf("stdout and stderr is closed")
	}()
	return
}

func startEmulator(emulatorPath string, args []string, androidHome string, runningDevices map[string]string, timeoutChan <-chan time.Time) (string, error) {
	log.TDonef("$ %s", strings.Join(append([]string{emulatorPath}, args...), " "))

	cmdOptions := asyncCmd.Options{Buffered: false, Streaming: true}
	cmd := asyncCmd.NewCmdOptions(cmdOptions, emulatorPath, args...)

	errChan := make(chan error)

	serialChan := checkDeviceSerial(androidHome, runningDevices, errChan)
	stdoutChan, stderrChan := broadcastStdoutAndStderr(cmd)
	go handleOutput(stdoutChan, stderrChan, errChan)

	select {
	case <-cmd.Start():
		log.Warnf("emulator exited unexpectedly")
		return startEmulator(emulatorPath, args, androidHome, runningDevices, timeoutChan)
	case err := <-errChan:
		log.Warnf("error occurred: %", err)
		if err := cmd.Stop(); err != nil {
			log.Warnf("Failed to terminate emulator command: %s", err)
		}
		log.Warnf("restarting emulator...")
		return startEmulator(emulatorPath, args, androidHome, runningDevices, timeoutChan)
	case serial := <-serialChan:
		return serial, nil
	case <-timeoutChan:
		log.Warnf("timeout")
		return "", fmt.Errorf("timeout")
	}
}
