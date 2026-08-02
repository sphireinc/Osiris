//go:build windows

package osiris

import (
	"fmt"
	"os"
	"os/exec"
)

func configureProcessTree(_ *exec.Cmd) {}

func terminateProcessTree(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return nil
	}
	return command.Process.Kill()
}

func processSignalName(_ error) string { return "" }

func signalForName(name string) (os.Signal, string, error) {
	if name == "interrupt" || name == "int" || name == "SIGINT" {
		return os.Interrupt, "SIGINT", nil
	}
	return nil, "", fmt.Errorf("unsupported signal %q on Windows", name)
}
