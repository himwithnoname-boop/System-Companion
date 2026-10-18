//go:build !windows

package main

import "context"

// Portable mode does not elevate privileges or install kernel sensor drivers.
// OS utilization is collected separately; privileged sensors remain unavailable.
func platformSensors(ctx context.Context, s Sample) {}
