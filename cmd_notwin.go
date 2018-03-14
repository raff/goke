// +build !windows

package main

import (
	"os"
	"os/exec"
)

func runCommand(line string) error {
	cmd := exec.Command("sh", "-c", line)

	if true { // should we have an option to ignore the output ?
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	return cmd.Run()
}
