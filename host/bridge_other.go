//go:build !windows

package main

import (
	"context"
	"fmt"
)

type noBridge struct{}

func (noBridge) Collect(ctx context.Context, s Sample) {}
func sensorProvider(ctx context.Context) (Provider, func() error) {
	return noBridge{}, func() error { return fmt.Errorf("this OS uses native sensors; no Windows sensor bridge is needed") }
}
