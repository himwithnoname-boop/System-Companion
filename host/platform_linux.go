package main

func newPlatformProvider() Provider { return &linuxPlatform{root: "/sys"} }
