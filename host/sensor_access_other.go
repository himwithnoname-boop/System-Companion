//go:build !windows

package main

import "fmt"

func sensorAccessAvailable() bool  { return false }
func sensorAccessRemembered() bool { return false }
func setSensorAccessRemembered(bool) error {
	return fmt.Errorf("remembered CPU access is a Windows setting")
}

func sensorAccessState() Sample {
	return Sample{"available": false, "enabled": false, "pending": false}
}
