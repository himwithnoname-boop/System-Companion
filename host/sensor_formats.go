package main

import (
	"encoding/binary"
	"math"
)

func decodeSMCTemperature(kind string, b []byte) (float64, bool) {
	var v float64
	switch {
	case kind == "flt " && len(b) >= 4:
		v = float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
	case kind == "sp78" && len(b) >= 2:
		v = float64(int16(binary.BigEndian.Uint16(b))) / 256
	case kind == "fp88" && len(b) >= 2:
		v = float64(binary.BigEndian.Uint16(b)) / 256
	default:
		return 0, false
	}
	return v, usableTemperature(v)
}
