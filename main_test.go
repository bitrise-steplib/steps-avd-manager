package main

import (
	"testing"
	"time"
)

func Test_runCommandWithHangTimeout(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		timeout time.Duration
		wantErr bool
	}{
		{
			name:    "Simple timeout",
			script:  `sleep 10`,
			timeout: 2 * time.Second,
			wantErr: true, // signal: terminated
		},
		{
			name: "Content on stdout resets timer",
			script: `
sleep 2
echo 1
sleep 2
echo 2
sleep 2
echo 3
sleep 2`,
			timeout: 5 * time.Second,
			wantErr: false,
		},
		{
			name: "Content on stdout resets timer",
			script: `
sleep 2
echo 1
sleep 2
echo 2
sleep 10`,
			timeout: 5 * time.Second,
			wantErr: true, // signal: terminated
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runCommandWithHangTimeout("bash", []string{"-c", tt.script}, nil, tt.timeout)
			if (err != nil) != tt.wantErr {
				t.Errorf("runCommandWithHangTimeout() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
