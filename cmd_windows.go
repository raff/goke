package main

import (
	"os"
	"os/exec"
	"syscall"
)

func runCommand(line string) error {
	cmd := exec.Command("sh", "-c", line)
	cmd.Stdin = os.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if true { // should we have an option to ignore the output ?
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	return cmd.Run()
}
