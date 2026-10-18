package main

import (
	"context"
	"github.com/shirou/gopsutil/v4/sensors"
	"strings"
)

func linuxTemperatures(ctx context.Context, s Sample) {
	if ts, e := sensors.TemperaturesWithContext(ctx); e == nil {
		for _, t := range ts {
			if strings.Contains(t.SensorKey, "coretemp") || strings.Contains(t.SensorKey, "k10temp") || strings.Contains(t.SensorKey, "cpu") {
				if t.Temperature > 0 {
					s["cpu_temp"] = t.Temperature
					break
				}
			}
		}
	}
}
