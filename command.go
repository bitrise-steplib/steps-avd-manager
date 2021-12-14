package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/bitrise-io/go-utils/log"
	asyncCmd "github.com/go-cmd/cmd"
)

func stopAfter(cmd *asyncCmd.Cmd, timeout time.Duration, doneChan <-chan struct{}) {
	select {
	case <-time.After(timeout):
		log.Warnf("timeout out")
		if err := cmd.Stop(); err != nil {
			log.Warnf("Failed to terminate command: %s", err)
		}
	case <-doneChan:
		return
	}
}

func stopAfterSilence(cmd *asyncCmd.Cmd, timeout time.Duration, stdoutChan, stderrChan chan string, doneChan <-chan struct{}) {
	for {
		select {
		case <-stdoutChan:
		case <-stderrChan:
		case <-time.After(timeout):
			log.Warnf("timeout out after silence")
			if err := cmd.Stop(); err != nil {
				log.Warnf("Failed to terminate command: %s", err)
			}
		case <-doneChan:
			return
		}
	}
}

func broadcastStdoutAndStderr(cmd *asyncCmd.Cmd, stdoutChan, stderrChan chan<- string, doneChan chan<- struct{}) {
	defer close(doneChan)
	// Done when both channels have been closed
	// https://dave.cheney.net/2013/04/30/curious-channels
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
}

func printOutput(stdoutChan, stderrChan <-chan string, doneChan <-chan struct{}) {
	for {
		select {
		case line := <-stdoutChan:
			fmt.Fprintln(os.Stdout, line)
		case line := <-stderrChan:
			fmt.Fprintln(os.Stderr, line)
		case <-doneChan:
			return
		}
	}
}

func run(name string, args []string, stdin io.Reader, silenceTimeout time.Duration, timeout time.Duration) error {
	cmdOptions := asyncCmd.Options{Buffered: false, Streaming: true}
	cmd := asyncCmd.NewCmdOptions(cmdOptions, name, args...)

	doneChan := make(chan struct{})
	stdoutChan := make(chan string)
	stderrChan := make(chan string)

	go stopAfter(cmd, timeout, doneChan)
	go stopAfterSilence(cmd, silenceTimeout, stdoutChan, stderrChan, doneChan)
	go printOutput(stdoutChan, stderrChan, doneChan)
	go broadcastStdoutAndStderr(cmd, stdoutChan, stderrChan, doneChan)

	// Run and wait for Cmd to return
	status := <-cmd.StartWithStdin(stdin)

	log.Warnf("cmd returned with status: %d, error: %s", status.Exit, status.Error)

	// Wait for goroutine to print everything
	<-doneChan

	log.Warnf("all output write is done")

	if status.Exit != 0 && status.Error == nil {
		return fmt.Errorf("exit status %d", status.Exit)
	}

	return status.Error
}
