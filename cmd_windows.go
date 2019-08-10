package main

import (
	"os"
	"os/exec"
	"syscall"
)

func runCommand(line string, out bool) error {
	cmd := exec.Command("sh", "-c", line)
	cmd.Stdin = os.Stdin
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if true { // should we have an option to ignore the output ?
		if !out {
			cmd.Stdout = os.Stdout
		}
		cmd.Stderr = os.Stderr
	}

	if out {
		res, err := cmd.Output()
		return string(res), err
	}

	return "", cmd.Run()

}
