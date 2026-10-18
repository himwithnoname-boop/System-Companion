# Client performance verification — 1.0.2

Historical development measurement on Windows with the connected ESP32, a 32-thread CPU and a discrete NVIDIA GPU. These are short real-machine measurements, not a guarantee of zero impact or results on other operating systems.

| Metric, 30-second steady-state window | Previous client | First optimized candidate | 1.0.2 release candidate | Installed 1.0.2 |
| --- | ---: | ---: | ---: | ---: |
| CPU time including GPU children | 1.296875 s | 0.109375 s | 0.218750 s | 0.093750 s |
| CPU, as percentage of one core | 4.314% | 0.364% | 0.728% | 0.312% |
| CPU, as percentage of this 32-thread machine | 0.1348% | 0.0114% | 0.0227% | 0.0097% |
| Child processes started during the window | 56 | 0 | 0 | 0 |
| Mean combined working set | 48.16 MiB | 66.25 MiB | 68.10 MiB | 80.61 MiB |
| Peak sampled combined working set | 85.82 MiB | 66.47 MiB | 68.13 MiB | 80.78 MiB |

The measured reduction in CPU time was about 83–93% across optimized runs. Average combined working set increased by about 18–32 MiB, while each sampled peak remained below the baseline. Keeping the GPU reader alive is a deliberate memory tradeoff for fewer launches. The installed client was measured running directly from the device's USB drive with both temperatures working; the separate CPU helper remains excluded as described below. Default live telemetry remains one second; ten observed samples had a maximum reception gap of 1.11 seconds during the initial browser check. The six-mode browser check observed eleven samples with a maximum gap of 1.097 seconds. Processes, disks, network, CPU load, RAM and GPU readings remained available.

## Method and limits

`tools/measure-client.py` uses a Windows Job Object with no limits or kill-on-close behavior. It accounts for CPU in the assigned monitor and its children, including short-lived GPU commands. For a persistent reader, assign its existing PID as well as the monitor PID. Every 200 ms it sums the working sets of currently active assigned processes. Shared pages can be counted twice; a sampled peak can miss brief spikes. The sampler itself and browser are outside the measured job.

The already enabled elevated CPU helper is excluded from the comparison. Its observed working set before the update was approximately 31 MiB. Optional CPU sensing adds resources when enabled. Measurements were sequential on a machine with other applications running, rather than a controlled workload benchmark. No gaming latency, battery life, or sustained-load benchmark was performed.

## Collection behavior

| Source | Default collection/retry behavior | Expiry |
| --- | --- | --- |
| CPU load, RAM, swap, network | About once per second in one worker | 4 seconds |
| NVIDIA GPU | One hidden `nvidia-smi` process sampling every second | 3 seconds |
| Storage I/O | About every 2 seconds; capacity and mounted-drive discovery every 30 seconds | 12 seconds |
| Top processes | About every 5 seconds; names resolved only for displayed candidates | 15 seconds |
| Windows WMI / Linux native temperatures | About every 2 seconds when available; 30 seconds when absent | 6 seconds |
| Optional Windows CPU helper | Every 2 seconds; stops after 3 unsuccessful temperature reads | 6 seconds from sensor timestamp |
| Local AI models | Every 5 seconds when detected; 30 seconds when absent | 12 seconds |
| Disconnected USB | Bounded retry delay, up to 5 seconds; manual reconnect retries immediately | Device separately marks lost telemetry |

Slow workers never overlap themselves. Their immutable cached maps are omitted after expiry, so old values do not masquerade as live readings. Native calls that ignore cancellation may occupy their one worker until returning; they do not accumulate additional workers or block the telemetry loop. GPU output is bounded, a silent reader is terminated, and failed readers retry after 30 seconds. Shutdown cancels and reaps the reader.

The dashboard pauses requests while hidden, avoids rendering an unchanged sample, backs off when the agent is unavailable, and stops requests after Exit or an invalid session. The agent continues monitoring independently of the browser. Binding the dashboard before starting providers prevents duplicate launch overhead.

The NVIDIA polling implementation follows its [documented loop option](https://docs.nvidia.com/deploy/nvidia-smi/); it does not change GPU configuration or install a driver. The client honors `--interval` between 500 ms and 5 seconds; GPU polling never runs faster than once per second.

## Validation

Go tests cover stuck collectors, cache expiry, missing-backend backoff, unavailable GPU values, protocol frame limits and local AI discovery. `go vet` checks the host code. The browser test covers six centre modes, tray registration, hidden-tab pause/resume, independent device telemetry, and stopping requests after Exit.

Live fault injection stopped only the owned GPU reader: its cached temperature disappeared after 3.45 seconds, and GPU readings recovered after 30.47 seconds. During the fault, 32 CPU/USB samples continued with a maximum reception gap of 1.094 seconds. Manual USB reconnect returned to COM9. Duplicate launches exited before starting providers.

All five Windows/Linux/macOS architecture builds compile with CGO disabled. Windows and Linux x64 are runtime-tested here. Linux ARM64 and macOS Intel/Apple Silicon still need resource and device tests on their respective machines; their sensor and tray limitations remain as documented in the main README.

Version 1.0.2 adds the sixth centre mode and optional Windows sign-in startup. Startup was enabled and disabled through the same backend used by the tray menu; the stable copied executable matched the release hash. With a forced absent port, startup exited successfully before starting collectors. With the real connected display, a quiet startup launch connected to COM9 and reported live readings. Startup remained disabled after the check. Display checks covered all six real input-path taps, Celsius, Fahrenheit, missing CPU temperature, and migration of existing brightness/auto-rotation preferences. Captured hardware temperatures in the layout test are explicitly controlled fixtures, not reported live measurements.

## Offline on-device package (1.0.2 measurements)

All five clients and optional Windows sensor libraries now fit in internal flash through offline compression. The FAT file allocation is 12,881,920 bytes, with space reserved for filesystem metadata in the 13 MiB partition. Every packaged file was read back from the physical USB volume and checked against the carried SHA-256 manifest. Windows uses a native GUI self-extractor with an explicit asInvoker manifest; no extraction progress or console window appears. It releases decoding buffers before waiting for the monitor and removes its temporary folder after clean tray Exit. Its observed working set while waiting was 7.61 MiB in addition to the Go client and sensor readers; the earlier Windows measurements above excluded this new portable wrapper.

Linux carries static x64 and ARM64 decoders (approximately 24 KiB each), using Linux syscalls with no C runtime or Android dependency. The x64 decoder was tested inside Ubuntu with both archived clients; their bytes matched the full releases. A corrupted archive failed without emitting executable data. Decoding and verification in that check took 0.318 seconds; this is a single VM observation, not a guaranteed launch time. Linux/macOS shell wrappers wait without polling and clean their private run directories after Exit. The decompressor runs only during launch. macOS's launcher and Linux ARM64 still require runtime validation on those architectures.

## Ubuntu x64 device check

The running four-vCPU BenchDesk Ubuntu VM launched `Linux/Monitor` directly from the physical USB volume, without a shared folder or client download. Serial access used a user-approved temporary ACL on `/dev/ttyACM0`. During a 30.509-second observation the client delivered 31 samples, with a maximum observed gap of 1.021 seconds. Its mean RSS was 13.71 MiB, peak sampled RSS 14.28 MiB, and CPU time was 3.048% of one core (about 0.762% of this four-vCPU guest).

CPU load, memory, network, drives and processes remained available. The guest exposed no CPU/GPU temperature sensors; these fields were absent. Dashboard Exit stopped the client and removed its private run directory. Firmware diagnostics then reported 86 accepted frames, zero rejected frames, automatic rotation enabled, and the saved stacked-temperature selection retained.

These process-only measurements read `/proc` every 200 ms. They exclude the browser, Python verifier, initial decoding and waiting shell wrapper; they cannot be compared directly with the Windows Job Object totals above. VM scheduling and background applications affect the result. Native Linux GPU and temperature providers, Linux ARM64, and macOS remain unmeasured. Evidence is saved in `dist/Verification/linux-runtime.json`, `linux-exit.json` and `device-diagnostics.json`.

## 1.0.3 storage and preference update

The new package combines all four Linux/macOS clients into one shared XZ archive (7,055,844 bytes), while Windows retains its GUI extractor (3,832,146 bytes). Total FAT file allocation is 11,079,680 bytes, plus filesystem metadata within the unchanged 13 MiB partition. All 29 packaged files were checked against the manifest from the physical device. The decoder's bounded output grows to 64 MiB for the shared archive; decoding runs only at launch and frees its buffers before monitoring. Those transient allocations are excluded from steady-state client measurements.

The approximately 63 GB SD card appears as a separate writable MoniStore device with FAT32 label MONISTORE. Its existing filesystem was relabeled, never reformatted. A temporary 128 KiB file passed write/flush/read verification and was removed; existing root entries remained unchanged. Writes to the internal client drive were rejected. See `monistore-windows.json`.

Device preferences arrive through the existing USB handshake and change messages, with no extra periodic serial requests. Browser checks verified saved settings and stacked centre restoration, no default preference writes on opening, preservation of unsent edits during polling, and sending only controls actually changed. Native Windows and Linux tests passed. Linux's D-Bus tray waits for desktop events without polling or spawning helper processes; startup remains an explicit option. Prior Windows and Linux performance tables describe 1.0.2 and should not be treated as new 1.0.3 measurements.

## 1.0.5 rotation and physical power controls

The rotation helper was compiled from the production firmware header and passed 95 checks covering all signed axes, a full circle of angles, flat/diagonal hold and invalid readings. The real sensor reference with the USB port at the bottom was approximately +1 g on X; the corrected firmware reported LVGL rotation 270 degrees. The remaining three physical sides still need user confirmation; their mappings passed the production C checks.

The physical-button helper passed 39 checks covering its ten-second threshold, cancelling on early release, contact debounce, triggering shutdown once and ignoring a held wake press until release. The user confirmed the two-second cancel, ten-second shutdown, BOOT wake and hardware RESET sequence. The automated observer did not capture those transitions during its observation window, so this manual confirmation is recorded separately rather than presented as measured shutdown/wake timing. Final diagnostics identified firmware 1.0.5 and confirmed saved preferences. Shutdown is ESP32 deep sleep, not a full electrical disconnection; its current draw has not been measured.

Only the application partition was flashed and its written hash verified. The client archive, NVS and SD filesystem were retained. Button polling runs in the existing firmware event loop; no host polling process or periodic subprocess was added. The temporary diagnostic observer exits before the portable monitoring client resumes. See `dist/Verification/release-1.0.5.json`.

## Local AI discovery, client 1.0.4

An initial Windows check during an existing Maestro image/video generation job measured the updated monitor process for 25.0 seconds: 0.687% of one logical CPU core and 69.8 MiB working set. This short sample excludes the persistent NVIDIA child and any optional CPU helper, and is not a cross-platform benchmark. The process scan is shared with process rankings; recognized worker identities/socket ports are cached for thirty seconds, AI APIs are sampled every five seconds by one bounded worker, and unavailable defaults retry after thirty seconds. No inference is started, no engine configuration is changed, and local telemetry bypasses proxies and redirects. Linux and macOS performance for this update remains unmeasured.
