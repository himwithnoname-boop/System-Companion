//go:build !darwin

package main

import (
	"go.bug.st/serial/enumerator"
	"strings"
)

func discoverPorts() []string {
	ports, e := enumerator.GetDetailedPortsList()
	if e != nil {
		return nil
	}
	var names []string
	for _, p := range ports {
		if p.IsUSB && (strings.EqualFold(p.VID, "303A") || strings.Contains(strings.ToLower(p.Product), "companion")) {
			names = append(names, p.Name)
		}
	}
	return names
}
