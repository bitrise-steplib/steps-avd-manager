package emuinstaller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bitrise-steplib/steps-avd-manager/test"
	"github.com/bitrise-io/go-utils/v2/command"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/stretchr/testify/require"
)

func TestIsVersionInstalled(t *testing.T) {
	tests := []struct {
		name          string
		buildNumber   string
		versionOutput string
		expectError   bool
		expectResult  bool
	}{
		{
			name:        "build number does not match",
			buildNumber: "12345",
			versionOutput: `INFO    | Storing crashdata in: /tmp/android-oliverfalvai/emu-crash-34.2.16.db, detection is enabled for process: 32611
INFO    | Android emulator version 34.2.16.0 (build_id 12038310) (CL:N/A)
INFO    | Storing crashdata in: /tmp/android-oliverfalvai/emu-crash-34.2.16.db, detection is enabled for process: 32611
INFO    | Duplicate loglines will be removed, if you wish to see each individual line launch with the -log-nofilter flag.
Android emulator version 34.2.16.0 (build_id 12038310) (CL:N/A)
Copyright (C) 2006-2024 The Android Open Source Project and many others.
This program is a derivative of the QEMU CPU emulator (www.qemu.org).

  This software is licensed under the terms of the GNU General Public
  License version 2, as published by the Free Software Foundation, and
  may be copied, distributed, and modified under those terms.

  This program is distributed in the hope that it will be useful,
  but WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
  GNU General Public License for more details.`,
			expectError:  false,
			expectResult: false,
		},
		{
			name:        "build number matches",
			buildNumber: "12038310",
			versionOutput: `INFO    | Storing crashdata in: /tmp/android-oliverfalvai/emu-crash-34.2.16.db, detection is enabled for process: 32611
INFO    | Android emulator version 34.2.16.0 (build_id 12038310) (CL:N/A)
INFO    | Storing crashdata in: /tmp/android-oliverfalvai/emu-crash-34.2.16.db, detection is enabled for process: 32611
INFO    | Duplicate loglines will be removed, if you wish to see each individual line launch with the -log-nofilter flag.
Android emulator version 34.2.16.0 (build_id 12038310) (CL:N/A)
Copyright (C) 2006-2024 The Android Open Source Project and many others.
This program is a derivative of the QEMU CPU emulator (www.qemu.org).

  This software is licensed under the terms of the GNU General Public
  License version 2, as published by the Free Software Foundation, and
  may be copied, distributed, and modified under those terms.

  This program is distributed in the hope that it will be useful,
  but WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
  GNU General Public License for more details.`,
			expectError:  false,
			expectResult: true,
		},
		{
			name:          "build number not found",
			buildNumber:   "12345",
			versionOutput: "",
			expectError:   true,
			expectResult:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdFactory := test.FakeCommandFactory{
				Stdout:   tt.versionOutput,
				ExitCode: 0,
			}

			installer := EmuInstaller{
				androidHome: "/fake/android/home",
				cmdFactory:  cmdFactory,
			}

			result, err := installer.isVersionInstalled(tt.buildNumber)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.expectResult, result)
		})
	}
}

func TestBackupEmuDir(t *testing.T) {
	tests := []struct {
		name         string
		prepare      func(androidHome string)
		expectBackup bool
	}{
		{
			name:         "no emulator dir in android home",
			prepare:      nil,
			expectBackup: false,
		},
		{
			name: "emulator dir in android home",
			prepare: func(androidHome string) {
				err := os.Mkdir(filepath.Join(androidHome, "emulator"), 0755)
				require.NoError(t, err)
			},
			expectBackup: true,
		},
		{
			name: "emulator dir and backup dir in android home",
			prepare: func(androidHome string) {
				err := os.Mkdir(filepath.Join(androidHome, "emulator"), 0755)
				require.NoError(t, err)

				err = os.Mkdir(filepath.Join(androidHome, backupDir), 0755)
				require.NoError(t, err)
			},
			expectBackup: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdFactory := command.NewFactory(env.NewRepository())
			androidHome := t.TempDir()
			installer := EmuInstaller{
				androidHome: androidHome,
				cmdFactory:  cmdFactory,
			}

			if tt.prepare != nil {
				tt.prepare(androidHome)
			}
			err := installer.backupEmuDir()
			require.NoError(t, err)

			_, err = os.Stat(filepath.Join(androidHome, "emulator"))
			require.ErrorIs(t, err, os.ErrNotExist)

			_, err = os.Stat(filepath.Join(androidHome, backupDir))
			if tt.expectBackup {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}
