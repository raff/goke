//go:build !windows
// +build !windows

package main

import (
	"os"
	"os/exec"
)

func runCommand(line string, out bool) (string, error) {
	cmd := exec.Command("sh", "-c", line)
	cmd.Stdin = os.Stdin

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
