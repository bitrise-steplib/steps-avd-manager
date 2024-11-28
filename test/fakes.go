package test

import (
	"fmt"
	"strings"

	"github.com/bitrise-io/go-utils/v2/command"
)

type FakeCommandFactory struct {
	Stdout   string
	ExitCode int
}

func (f FakeCommandFactory) Create(name string, args []string, _ *command.Opts) command.Command {
	return fakeCommand{
		command:  fmt.Sprintf("%s %s", name, strings.Join(args, " ")),
		stdout:   f.Stdout,
		exitCode: f.ExitCode,
	}
}

type fakeCommand struct {
	command  string
	stdout   string
	stderr   string
	exitCode int
}

func (c fakeCommand) PrintableCommandArgs() string {
	return c.command
}

func (c fakeCommand) Run() error {
	if c.exitCode != 0 {
		return fmt.Errorf("exit code %d", c.exitCode)
	}
	return nil
}

func (c fakeCommand) RunAndReturnExitCode() (int, error) {
	if c.exitCode != 0 {
		return c.exitCode, fmt.Errorf("exit code %d", c.exitCode)
	}
	return c.exitCode, nil
}

func (c fakeCommand) RunAndReturnTrimmedOutput() (string, error) {
	if c.exitCode != 0 {
		return "", fmt.Errorf("exit code %d", c.exitCode)
	}
	return c.stdout, nil
}

func (c fakeCommand) RunAndReturnTrimmedCombinedOutput() (string, error) {
	if c.exitCode != 0 {
		return "", fmt.Errorf("exit code %d", c.exitCode)
	}
	return fmt.Sprintf("%s%s", c.stdout, c.stderr), nil
}

func (c fakeCommand) Start() error {
	return nil
}

func (c fakeCommand) Wait() error {
	return nil
}
