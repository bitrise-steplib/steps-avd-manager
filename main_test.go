package main

import (
	"testing"
	"time"
)

func Test_runCommandWithHangTimeout(t *testing.T) {
	tests := []struct {
		name           string
		script         string
		silenceTimeout time.Duration
		timeout        time.Duration
		wantErr        bool
	}{
		{
			name:    "Simple timeout",
			script:  `sleep 10`,
			timeout: 2 * time.Second,
			wantErr: true, // signal: terminated
		},
		{
			name: "Content on stdout resets silence timer",
			script: `
sleep 2
echo 1
sleep 2
echo 2
sleep 2
echo 3
sleep 2`,
			silenceTimeout: 5 * time.Second,
			wantErr:        false,
		},
		{
			name: "If silence timeout goes by it fails",
			script: `
sleep 2
echo 1
sleep 2
echo 2
sleep 10`,
			silenceTimeout: 5 * time.Second,
			wantErr:        true, // signal: terminated
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.silenceTimeout == 0 {
				tt.silenceTimeout = 1 * time.Hour
			}
			if tt.timeout == 0 {
				tt.timeout = 1 * time.Hour
			}
			err := run("bash", []string{"-c", tt.script}, nil, tt.silenceTimeout, tt.timeout)
			if (err != nil) != tt.wantErr {
				t.Errorf("runCommandWithHangTimeout() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
