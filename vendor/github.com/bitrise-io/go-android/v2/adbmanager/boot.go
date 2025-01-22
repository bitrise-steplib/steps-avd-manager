package adbmanager

import (
	"fmt"
	"strings"
	"time"
)

type WaitForBootCompleteResult struct {
	// Booted is true if the device is online AND it's booted. A false value doesn't
	// necessarily mean an error, a retry might resolve things.
	Booted bool
	// Error signals a non-retryable problem.
	Error  error
}

func (model *Model) getBootCompleteEvent(serial string, timeout time.Duration) <-chan WaitForBootCompleteResult {
	doneChan := make(chan WaitForBootCompleteResult)

	go func() {
		time.AfterFunc(timeout, func() {
			doneChan <- WaitForBootCompleteResult{Error: fmt.Errorf("timeout while waiting for boot complete event")}
		})
	}()

	go func() {
		cmd := model.WaitForDeviceThenShellCmd(serial, nil, "getprop sys.boot_completed")
		out, err := cmd.RunAndReturnTrimmedCombinedOutput()
		if err != nil {
			fmt.Println(cmd.PrintableCommandArgs())
			fmt.Println(out)
			doneChan <- WaitForBootCompleteResult{Error: err}
			return
		}

		lines := strings.Split(out, "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "1" {
				doneChan <- WaitForBootCompleteResult{Booted: true}
				return
			}
		}

		doneChan <- WaitForBootCompleteResult{Booted: false}
	}()

	return doneChan
}
