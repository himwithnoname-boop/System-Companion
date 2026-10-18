package main

import (
	"go.bug.st/serial"
	"strings"
)

// macOS USB serial enumeration uses the native callout device names without cgo.
// Identity is always verified through the firmware's protocol handshake.
func discoverPorts() []string {
	ports, e := serial.GetPortsList()
	if e != nil {
		return nil
	}
	var names []string
	for _, p := range ports {
		if strings.HasPrefix(p, "/dev/cu.usbmodem") {
			names = append(names, p)
		}
	}
	return names
}
