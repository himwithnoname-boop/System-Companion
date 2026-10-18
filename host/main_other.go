//go:build !darwin

package main

func platformMain(run func()) { run() }
