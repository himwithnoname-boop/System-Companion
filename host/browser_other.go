//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

func openBrowser(url string) {
	command := "xdg-open"
	if runtime.GOOS == "darwin" {
		command = "open"
	}
	c := exec.Command(command, url)
	hideCommand(c)
	if c.Start() == nil {
		go c.Wait()
	}
}
