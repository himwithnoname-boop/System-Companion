# USB protocol v1

Device VID `303A`, PID `4002`: CDC telemetry and MSC storage. ROM programming uses PID `0009` or `1001`. Verify a `hello` response before sending telemetry; vendor ID alone does not identify the application.

Messages are UTF-8 JSON with newline delimiters, shorter than 8192 bytes. CR is ignored. Invalid JSON, oversized frames and versions other than 1 are rejected. Missing/null fields mean unavailable, distinct from measured zero.

```json
{"type":"hello","v":1}
{"type":"telemetry","v":1,"seq":1,"cpu_load":12.4,"gpu_temp":35,"gpu_load":2,"ram_used":16.2,"ram_total":125.5,"net_down":2.1,"net_up":0.4}
{"type":"settings","v":1,"theme":0,"brightness":170,"fahrenheit":false,"auto_rotate":true,"center_mode":0,"page":0,"timeout":120}
{"type":"diagnostics","v":1}
```

Wire units: Celsius, percent load, GiB memory/VRAM, MHz clocks, watts power, and Mbps network. `time` is the host local clock in seconds for the firmware's UTC formatter; `seq` identifies snapshots. Each frame replaces the current snapshot. Omitted fields do not retain old readings. Six seconds without a sample marks the connection stale.

Other fields are in `firmware/main/companion.h`. Arrays are bounded to 32 cores, 6 drives, and 10 processes. Drive entries provide name, total/used GiB, optional temperature/health and read/write MB/s. Process entries provide name, CPU percent and RAM GiB. AI fields report only what a provider supplies.

Pages: 0 overview, 1 CPU, 2 GPU, 3 memory, 4 storage, 5 network, 6 processes, 7 AI, 8 settings. Themes: 0 AMOLED, 1 NVIDIA, 2 Cyberpunk, 3 Light, 4 Matrix. Centre modes: 0 temperature, 1 CPU, 2 GPU, 3 memory, 4 network, 5 stacked CPU/GPU temperatures. Centre selection and display settings persist in NVS. Brightness is 20–255; timeout 0–3600 seconds, with zero keeping the display awake.

`capture` returns `frame_begin`, ordered base64 `frame_data`, and `frame_end`; pixels use RGB565 in display byte order. `tools/device.py` validates offsets and creates PNGs. Diagnostic `touch` uses physical coordinates, with state 1 pressed, 0 released, or -1 returning to real touch. `bootloader` acknowledges and enters ROM download mode; the build/reset tools resume the app.

Device `action` messages support refresh and reconnect. The host samples normally every second, with independent bounded collectors. Chart history tracks elapsed seconds and marks missed samples as gaps. The internal agent volume is read-only; inserted microSD storage is writable.

Firmware 1.0.3 adds a `settings` object to the `hello` response, containing theme, brightness, Fahrenheit, automatic rotation, centre mode, timeout and current page. The device also sends `{"type":"settings","settings":{...}}` after a saved change. Clients consume these values without sending their defaults on connection. `/api/status` includes the current device preferences and authenticated `GET /api/settings` returns them; preferences are cleared on disconnect.

Firmware 1.0.5 includes `power_hold_ms` and the ESP-IDF `wake_reason` in diagnostics. BOOT shutdown is a physical-button operation; ordinary telemetry and settings commands cannot shut the device down. A completed ten-second hold drains current device-side SD I/O, stops serial captures and USB, sleeps the panel/accelerometer, then enters deep sleep after button release. BOOT wakes it through GPIO0 EXT0. Computer-side filesystem caches must be safely ejected before power-off.

MSC exposes two LUNs: 0 is the internal read-only client volume, and 1 is optional writable **MoniStore**. LUN 1 reports no medium when absent and unit attention after insertion. FAT cards receive label **MONISTORE** before host access. Unsupported or unformatted cards are exposed without automatic formatting. `hello` adds `sd_ready`, `sd_bytes`, `sd_name` and `sd_label` for diagnostics.

Firmware 1.0.6 accepts optional AI fields `ai_state`, `ai_provider`, `ai_model`, `ai_note`, `ai_cpu` (whole-machine CPU percent for the selected worker), `ai_ram` (worker RSS GiB), `ai_queue` (reported queued requests/jobs), `ai_tps` (observed generated tokens/sec), `ai_context` (engine KV cache percentage) and `ai_vram` (model-specific GiB). States are `ACTIVE`, `QUEUED`, `IDLE`, `LOADED`, `WORKER`, and `NO SOURCE`. Absence of activity instrumentation does not imply idle; stale/disconnected display readings show OFFLINE. New fields remain optional for protocol v1.

## Per-adapter graphics telemetry (client 1.0.6 / firmware 1.0.8)

Optional `gpus` is an array of records: `id`, `name`, `vendor`, `source`, `pci`/`luid` where exposed, and available numeric `temp`, `load`, `power`, `vram`, `vram_total`, `hotspot`, `clock`, `mem_clock`, `fan`, `fan_rpm`. Temperature is Celsius, load/fan percent, power watts, memory GiB, clocks MHz and fan speed RPM. `power_percent` is a separate driver percentage and must not be displayed as watts. Absent/invalid values remain unavailable. The dashboard keeps the full array; the serial transport caps each batch at eight adapters and rotates it for larger inventories, with `gpu_count` total and zero-based `gpu_offset`. Names/identities are compacted only on the device wire.

Legacy `gpu_*` fields remain summaries for old firmware: hottest temperature/hotspot, mean load across reporting adapters and known memory/power totals (possibly partial). Records are joined by strong identity/PCI/LUID before a unique-name fallback. Two identically named physical adapters remain distinct.

`cpu_temp_source` and `cpu_temp_kind` describe permitted CPU readings. `thermal_temp`/`thermal_name` describe a separately labelled thermal-zone fallback; generic zones do not become CPU die readings. Detailed `thermal_zones` remain dashboard-side to preserve the 8191-byte frame budget. Settings and the protocol version remain unchanged.

## GPU visibility and all-GPU activity (1.0.7 / 1.0.9)

The client hello includes a 32-character lowercase hex `host_key`. Settings reports advertise `gpu_selection: true` and return `host_key` plus `gpu_hidden`, an array of up to sixteen 32-character GPU identity hashes. Settings writes may update `gpu_hidden` with the matching host key. Firmware atomically stores four bounded host profiles in NVS, independently of the existing theme preferences. Only explicit customisation changes profile recency; merely connecting another computer does not evict a saved profile. An empty array shows all adapters. Invalid or unsaved selections return a protocol error. The client waits for a matching settings report before the HTTP settings request succeeds.

Dashboard telemetry keeps the full `gpus` inventory, stable `display_key` hashes and index labels (GPU0 integrated, GPU1 onward discrete/other; GPU0 absent if no integrated adapter is identified). The serial detail array is filtered and its GPU load summary follows the selection. `gpu_detected` is the full inventory count, `gpu_selected` the visible count, `gpu_count`/`gpu_offset` describe the visible wire batches. On the serial link, `gpu_load` is the mean of selected reporting adapters; `gpu_load_available` counts contributors and `gpu_load_method` is `mean_selected_available`. Zero is valid; missing loads and disabled GPUs are excluded. All hidden or no reporting selected adapters produces null load. The dashboard computes the same selected mean from the full inventory, including during preference previews. Overview detail text and the graphics page use selected records. Overall temperature alerts retain all-GPU monitoring.
