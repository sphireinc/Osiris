//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package osiris

import (
	"fmt"
	"os/exec"
	"syscall"
)

func configureProcessTree(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcessTree(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return nil
	}
	if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err == nil {
		return nil
	}
	return command.Process.Kill()
}

func processSignalName(err error) string {
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return status.Signal().String()
		}
	}
	return ""
}

func signalForName(name string) (syscall.Signal, string, error) {
	switch name {
	case "interrupt", "int", "SIGINT":
		return syscall.SIGINT, "SIGINT", nil
	case "terminate", "term", "SIGTERM":
		return syscall.SIGTERM, "SIGTERM", nil
	case "hangup", "hup", "SIGHUP":
		return syscall.SIGHUP, "SIGHUP", nil
	case "user1", "usr1", "SIGUSR1":
		return syscall.SIGUSR1, "SIGUSR1", nil
	case "user2", "usr2", "SIGUSR2":
		return syscall.SIGUSR2, "SIGUSR2", nil
	default:
		return 0, "", fmt.Errorf("unsupported signal %q", name)
	}
}
