//go:build !linux

package main

import "context"

func linuxTemperatures(ctx context.Context, s Sample) {}
