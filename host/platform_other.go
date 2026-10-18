//go:build !windows && !darwin && !linux

package main

import "context"

type otherPlatform struct{}

func newPlatformProvider() Provider                   { return otherPlatform{} }
func (otherPlatform) Collect(context.Context, Sample) {}
