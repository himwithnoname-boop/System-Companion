//go:build !windows

package main

import (
	"os/exec"
)

func hideCommand(c *exec.Cmd) {}
