package adbmanager

import (
	"fmt"
	"path/filepath"

	"github.com/bitrise-io/go-android/sdk"
	"github.com/bitrise-io/go-utils/pathutil"
	"github.com/bitrise-io/go-utils/v2/command"
)

// Model ...
type Model struct {
	binPth     string
	cmdFactory command.Factory
}

// New ...
func New(sdk sdk.AndroidSdkInterface, cmdFactory command.Factory) (*Model, error) {
	binPth := filepath.Join(sdk.AndroidHome(), "platform-tools", "adb")
	if exist, err := pathutil.IsPathExists(binPth); err != nil {
		return nil, fmt.Errorf("failed to check if adb exist, error: %s", err)
	} else if !exist {
		return nil, fmt.Errorf("adb not exist at: %s", binPth)
	}

	return &Model{
		binPth:     binPth,
		cmdFactory: cmdFactory,
	}, nil
}

// DevicesCmd ...
func (model Model) DevicesCmd() *command.Command {
	cmd := model.cmdFactory.Create(model.binPth, []string{"devices"}, nil)
	return &cmd
}

// UnlockDevice ...
func (model Model) UnlockDevice(serial string) error {
	keyEvent82Cmd := model.cmdFactory.Create(model.binPth, []string{"-s", serial, "shell", "input", "82", "&"}, nil)
	if err := keyEvent82Cmd.Run(); err != nil {
		return err
	}

	keyEvent1Cmd := model.cmdFactory.Create(model.binPth, []string{"-s", serial, "shell", "input", "1", "&"}, nil)
	return keyEvent1Cmd.Run()
}
