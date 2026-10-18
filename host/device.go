package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"go.bug.st/serial"
	"io"
	"strings"
	"sync"
	"time"
)

type Device struct {
	mu                sync.Mutex
	port              serial.Port
	name              string
	forced            string
	action            chan string
	wake              chan struct{}
	reconnect         chan struct{}
	generation        uint64
	settings          *DisplaySettings
	settingsMu        sync.Mutex
	settingsChanged   chan struct{}
	settingsRevision  uint64
	lastSettingsError string
}

// Preferences belong to the display, never to a particular host's defaults.
type DisplaySettings struct {
	Theme        int      `json:"theme"`
	Brightness   int      `json:"brightness"`
	Fahrenheit   bool     `json:"fahrenheit"`
	AutoRotate   bool     `json:"auto_rotate"`
	CenterMode   int      `json:"center_mode"`
	Timeout      int      `json:"timeout"`
	Page         int      `json:"page"`
	GPUSelection bool     `json:"gpu_selection"`
	HostKey      string   `json:"host_key"`
	GPUHidden    []string `json:"gpu_hidden"`
}

func (s *DisplaySettings) valid() bool {
	return s != nil && s.Theme >= 0 && s.Theme < 5 && s.Brightness >= 20 && s.Brightness <= 255 && s.CenterMode >= 0 && s.CenterMode < 6 && s.Timeout >= 0 && s.Timeout <= 3600 && s.Page >= 0 && s.Page < 9
}
func (d *Device) Preferences() *DisplaySettings {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.settings == nil {
		return nil
	}
	s := *d.settings
	s.GPUHidden = append([]string{}, d.settings.GPUHidden...)
	return &s
}

func (d *Device) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.generation++
	d.settings = nil
	if d.port != nil {
		d.port.Close()
		d.port = nil
		d.name = ""
	}
}
func (d *Device) Reconnect() {
	d.Close()
	select {
	case d.reconnect <- struct{}{}:
	default:
	}
}

func (d *Device) maintain(ctx context.Context) {
	delay := time.Second
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.reconnect:
			delay = time.Second
		case <-timer.C:
		}
		if d.Name() == "" {
			if d.Connect(ctx) != nil {
				delay = min(2*delay, 5*time.Second)
			} else {
				delay = time.Second
			}
		} else {
			delay = time.Second
		}
		timer.Reset(delay)
	}
}
func (d *Device) Name() string { d.mu.Lock(); defer d.mu.Unlock(); return d.name }
func (d *Device) candidates() []string {
	if d.forced != "" {
		return []string{d.forced}
	}
	return discoverPorts()
}
func (d *Device) Connect(ctx context.Context) error {
	d.mu.Lock()
	generation := d.generation
	d.mu.Unlock()
	for _, name := range d.candidates() {
		p, e := serial.Open(name, &serial.Mode{BaudRate: 115200})
		if e != nil {
			continue
		}
		p.SetReadTimeout(500 * time.Millisecond)
		helloRequest, _ := json.Marshal(Sample{"type": "hello", "v": 1, "host_key": clientHostKey()})
		p.Write(append(helloRequest, '\n'))
		start := time.Now()
		data := make([]byte, 0, 2048)
		buf := make([]byte, 512)
		verified := false
		var preferences *DisplaySettings
		for time.Since(start) < 2200*time.Millisecond && ctx.Err() == nil {
			n, e := p.Read(buf)
			if e != nil {
				break
			}
			data = append(data, buf[:n]...)
			if len(data) > 8192 {
				break
			}
			for {
				i := strings.IndexByte(string(data), '\n')
				if i < 0 {
					break
				}
				var hello struct {
					Type     string           `json:"type"`
					Device   string           `json:"device"`
					V        int              `json:"v"`
					Settings *DisplaySettings `json:"settings"`
				}
				json.Unmarshal(data[:i], &hello)
				data = data[i+1:]
				if hello.Type == "hello" && hello.Device == "system-companion" && hello.V == 1 {
					verified = true
					if hello.Settings.valid() {
						preferences = hello.Settings
					}
					break
				}
			}
			if verified {
				break
			}
		}
		if !verified {
			p.Close()
			continue
		}
		p.SetReadTimeout(time.Second)
		d.mu.Lock()
		if ctx.Err() != nil || d.generation != generation || d.port != nil {
			d.mu.Unlock()
			p.Close()
			return errors.New("USB connection superseded")
		}
		d.port = p
		d.name = name
		d.settings = preferences
		d.settingsSignalLocked()
		d.mu.Unlock()
		go d.read(p)
		return nil
	}
	return errors.New("waiting for System Companion USB device")
}

type timeoutReader struct{ p serial.Port }

func (r timeoutReader) Read(b []byte) (int, error) {
	for {
		n, e := r.p.Read(b)
		if n > 0 || e != nil {
			return n, e
		}
	}
}
func (d *Device) read(p serial.Port) {
	scan := bufio.NewScanner(timeoutReader{p})
	scan.Buffer(make([]byte, 2048), 8192)
	for scan.Scan() {
		var m struct {
			Type     string           `json:"type"`
			Action   string           `json:"action"`
			Error    string           `json:"error"`
			Settings *DisplaySettings `json:"settings"`
		}
		if json.Unmarshal(scan.Bytes(), &m) != nil {
			continue
		}
		if m.Type == "settings" && m.Settings.valid() {
			d.mu.Lock()
			if d.port == p {
				d.settings = m.Settings
				d.settingsSignalLocked()
			}
			d.mu.Unlock()
		}
		if m.Type == "error" && m.Error != "" {
			d.mu.Lock()
			if d.port == p {
				d.lastSettingsError = m.Error
				d.settingsSignalLocked()
			}
			d.mu.Unlock()
		}
		if m.Type == "action" {
			select {
			case d.action <- m.Action:
			default:
			}
			select {
			case d.wake <- struct{}{}:
			default:
			}
		}
	}
	d.mu.Lock()
	if d.port == p {
		d.port = nil
		d.name = ""
		d.settings = nil
	}
	d.mu.Unlock()
	p.Close()
}
func (d *Device) Send(v any) error {
	b, e := json.Marshal(serialTelemetry(displayTelemetry(v, d.Preferences())))
	if e != nil {
		return e
	}
	if len(b) >= 8191 {
		return errors.New("telemetry exceeds protocol frame limit")
	}
	b = append(b, '\n')
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.port == nil {
		return io.ErrClosedPipe
	}
	n, e := d.port.Write(b)
	if e != nil || n != len(b) {
		d.port.Close()
		d.port = nil
		d.name = ""
		d.settings = nil
		if e == nil {
			e = io.ErrShortWrite
		}
	}
	return e
}
