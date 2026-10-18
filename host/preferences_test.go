package main

import "testing"

func TestDevicePreferencesAreValidatedAndCopied(t *testing.T) {
	original := &DisplaySettings{Theme: 2, Brightness: 255, AutoRotate: true, CenterMode: 5, Timeout: 120}
	if !original.valid() {
		t.Fatal("saved device preferences rejected")
	}
	d := &Device{settings: original}
	copy := d.Preferences()
	copy.Theme = 0
	if d.Preferences().Theme != 2 {
		t.Fatal("host mutated stored device preferences")
	}
	for _, bad := range []*DisplaySettings{nil, {Theme: 5, Brightness: 255}, {Brightness: 19}, {Brightness: 255, CenterMode: 6}, {Brightness: 255, Timeout: 3601}} {
		if bad.valid() {
			t.Fatalf("invalid preference accepted: %+v", bad)
		}
	}
	d.Close()
	if d.Preferences() != nil {
		t.Fatal("disconnected display left stale preferences")
	}
}
