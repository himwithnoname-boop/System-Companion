//go:build !windows && !linux && !darwin

package main

import "fmt"

func startupAvailable() bool { return false }
func startupEnabled() bool   { return false }
func setStartup(bool) error  { return fmt.Errorf("startup setting is currently available on Windows") }
