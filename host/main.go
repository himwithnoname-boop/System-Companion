package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

//go:embed dashboard.html
var dashboard string
var version = "1.0.9"

type State struct {
	mu     sync.RWMutex
	sample Sample
	err    string
	port   string
	json   []byte
}

func main() { platformMain(runMonitor) }

func runMonitor() {
	port := flag.String("port", "", "specific serial port; default auto discovery")
	noBrowser := flag.Bool("no-browser", false, "run without opening dashboard")
	once := flag.Bool("sample", false, "print one real telemetry sample and exit")
	startup := flag.Bool("startup", false, "start quietly only when the display is connected")
	addr := flag.String("listen", "127.0.0.1:8765", "loopback dashboard address")
	interval := flag.Duration("interval", time.Second, "telemetry interval (500msâ€“5s)")
	flag.Parse()
	if *interval < 500*time.Millisecond || *interval > 5*time.Second {
		log.Fatal("interval must be between 500ms and 5s")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *once {
		providers, closeProviders := startProviders(ctx, *interval)
		defer closeProviders()
		time.Sleep(2 * time.Second)
		b, _ := json.MarshalIndent(collect(ctx, providers, 1), "", "  ")
		fmt.Println(string(b))
		return
	}
	hostname, _, e := net.SplitHostPort(*addr)
	if e != nil || (hostname != "127.0.0.1" && hostname != "localhost" && hostname != "::1") {
		log.Fatal("dashboard must bind to loopback")
	}
	listener, e := net.Listen("tcp", *addr)
	if e != nil {
		log.Fatalf("dashboard unavailable (another agent may already be running): %v", e)
	}
	// Bind first so duplicate launches create no collectors or sensor helpers.
	defer listener.Close()
	device := &Device{forced: *port, action: make(chan string, 8), wake: make(chan struct{}, 1), reconnect: make(chan struct{}, 1)}
	defer device.Close()
	if *startup {
		*noBrowser = true
		probe, stop := context.WithTimeout(ctx, 4*time.Second)
		e := device.Connect(probe)
		stop()
		if e != nil {
			return
		}
	}
	providers, closeProviders := startProviders(ctx, *interval)
	defer closeProviders()
	sensor, enableSensors := sensorProvider(ctx)
	providers = append(providers, startCached(ctx, sensor, 2*time.Second, 6*time.Second, 2*time.Second))
	tokenBytes := make([]byte, 24)
	if _, e = rand.Read(tokenBytes); e != nil {
		log.Fatal(e)
	}
	token := hex.EncodeToString(tokenBytes)
	go device.maintain(ctx)
	state := &State{err: "Waiting for USB device"}
	state.json, _ = json.Marshal(Sample{"version": version, "port": "", "error": state.err, "telemetry": nil})
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, dashboard)
	})
	authorized := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		state.mu.RLock()
		data := state.json
		state.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(data)
	})
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(device.Preferences())
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var s struct {
			Theme      *int      `json:"theme"`
			Brightness *int      `json:"brightness"`
			Fahrenheit *bool     `json:"fahrenheit"`
			Page       *int      `json:"page"`
			Timeout    *int      `json:"timeout"`
			AutoRotate *bool     `json:"auto_rotate"`
			CenterMode *int      `json:"center_mode"`
			GPUHidden  *[]string `json:"gpu_hidden"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&s) != nil {
			http.Error(w, "Invalid settings", 400)
			return
		}
		m := Sample{"type": "settings", "v": 1}
		if s.Theme != nil {
			if *s.Theme < 0 || *s.Theme > 4 {
				http.Error(w, "Invalid theme", 400)
				return
			}
			m["theme"] = *s.Theme
		}
		if s.Brightness != nil {
			if *s.Brightness < 20 || *s.Brightness > 255 {
				http.Error(w, "Invalid brightness", 400)
				return
			}
			m["brightness"] = *s.Brightness
		}
		if s.Page != nil {
			if *s.Page < 0 || *s.Page > 8 {
				http.Error(w, "Invalid page", 400)
				return
			}
			m["page"] = *s.Page
		}
		if s.Fahrenheit != nil {
			m["fahrenheit"] = *s.Fahrenheit
		}
		if s.AutoRotate != nil {
			m["auto_rotate"] = *s.AutoRotate
		}
		if s.CenterMode != nil {
			if *s.CenterMode < 0 || *s.CenterMode > 5 {
				http.Error(w, "Invalid centre mode", 400)
				return
			}
			m["center_mode"] = *s.CenterMode
		}
		if s.Timeout != nil {
			if *s.Timeout < 0 || *s.Timeout > 3600 {
				http.Error(w, "Invalid timeout", 400)
				return
			}
			m["timeout"] = *s.Timeout
		}
		if s.GPUHidden != nil {
			prefs := device.Preferences()
			if prefs == nil || !prefs.GPUSelection {
				http.Error(w, "Update the display firmware to save GPU selections", 409)
				return
			}
			if !validGPUKeys(*s.GPUHidden) {
				http.Error(w, "Invalid GPU selection (maximum 16 hidden adapters)", 400)
				return
			}
			m["gpu_hidden"] = *s.GPUHidden
			m["host_key"] = clientHostKey()
		}
		applyContext, stopApply := context.WithTimeout(r.Context(), 3*time.Second)
		defer stopApply()
		if e := device.ApplySettings(applyContext, m); e != nil {
			http.Error(w, "Device unavailable", 503)
			return
		}
		state.mu.Lock()
		state.json, _ = json.Marshal(Sample{"version": version, "port": state.port, "error": state.err, "telemetry": state.sample, "settings": device.Preferences()})
		state.mu.Unlock()
		w.WriteHeader(204)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go server.Serve(listener)
	url := "http://" + listener.Addr().String() + "/#" + token
	trayReady := startTray(ctx, cancel, url, device.Reconnect, func() {
		if e := enableSensors(); e != nil {
			log.Print(e)
		}
	})
	mux.HandleFunc("/api/sensors", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		if e := enableSensors(); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("/api/sensor-access", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sensorAccessState())
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var request struct {
			Enabled *bool `json:"enabled"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&request) != nil || request.Enabled == nil {
			http.Error(w, "enabled must be true or false", 400)
			return
		}
		if err := setSensorAccessRemembered(*request.Enabled); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.WriteHeader(202)
	})
	mux.HandleFunc("/api/exit", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		w.WriteHeader(204)
		cancel()
	})
	mux.HandleFunc("/api/lifecycle", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Sample{"tray": trayReady})
	})
	mux.HandleFunc("/api/reconnect", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		device.Reconnect()
		w.WriteHeader(204)
	})
	mux.HandleFunc("/api/startup", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Sample{"available": startupAvailable(), "enabled": startupEnabled()})
			return
		}
		if r.Method != "POST" {
			http.Error(w, "GET or POST required", 405)
			return
		}
		var option struct {
			Enabled *bool `json:"enabled"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&option) != nil || option.Enabled == nil {
			http.Error(w, "enabled is required", 400)
			return
		}
		if err := setStartup(*option.Enabled); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.WriteHeader(204)
	})
	log.Printf("System Companion %s Â· %s", version, url)
	if !*noBrowser {
		openBrowser(url)
	}
	seq := uint64(0)
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		var err error
		for {
			select {
			case action := <-device.action:
				if action == "reconnect" {
					device.Reconnect()
				}
			default:
				goto actionsDone
			}
		}
	actionsDone:
		seq++
		sample := collect(ctx, providers, seq)
		if device.Name() != "" {
			err = device.Send(sample)
		} else {
			err = fmt.Errorf("waiting for System Companion USB device")
		}
		state.mu.Lock()
		state.sample = sample
		state.port = device.Name()
		if err != nil {
			state.err = err.Error()
		} else {
			state.err = ""
		}
		state.json, _ = json.Marshal(Sample{"version": version, "port": state.port, "error": state.err, "telemetry": sample, "settings": device.Preferences()})
		state.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-ticker.C:
		case <-device.wake:
		}
	}
	shutdown, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	server.Shutdown(shutdown)
}
