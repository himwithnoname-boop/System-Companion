# Verification and support boundaries

Public package: **client 1.0.9 / firmware 1.0.11**, prepared on 18 October 2026.

## This revision

- All five native clients compile: Windows x64/ARM64, Linux x64/ARM64 and macOS ARM64.
- The Go test suite and Windows vet pass.
- Installer worker tests cover corrupted images, unsafe layouts/protected regions, incompatible chip/flash and changed USB identity.
- The production LCD-ID probe passes all 256 possible response values, command-bit, reset and clock-timing tests.
- ESP-IDF 5.5.5 builds the firmware and client filesystem. All five clients and distribution notices fit the 13 MiB client partition.
- The installer builder verifies its isolated bundled Python/esptool runtime and embedded image hashes. The final standalone EXE also passed its offline self-test (exit 0).
- Dashboard fixture tests exercise GPU selection, selected-load averaging, missing readings, safe device names and mobile layout.

**The replacement LCD-ID helper in firmware 1.0.11 has build/protocol-test coverage; this exact revision has not yet completed a maintainer-observed hardware flash-and-display check.** The user is handling the next board flash. Do not interpret the earlier physical tests below as a hardware certification of every subsequent revision.

## Physical tests from the preceding release

Client 1.0.8 / firmware 1.0.10 was tested on the supported Waveshare 1.43-B board with Windows x64:

- The standalone installer wrote and hash-verified all four image regions, then verified application boot.
- All 29 files on the USB client drive matched the packaged hashes; settings and the existing MoniStore card were preserved.
- The packaged Windows client launched with a working native tray, local dashboard and telemetry, without visible command windows.
- GPU1-only at 99% matched the device's 99% GPU activity bar; the full adapter inventory remained available.
- CPU readings worked through previously approved optional helper access.
- Closing the browser left monitoring running. Applied settings synchronized and unapplied drafts survived polling.
- USB framebuffer captures verified stacked temperatures and multi-GPU summaries.

These checks establish a tested baseline. They are not a universal performance benchmark, antivirus clearance, signing claim or a guarantee of sensor availability.

## Platform matrix

| Target | Build coverage | Physical runtime coverage |
| --- | --- | --- |
| Windows x64 | Current build and tests | Previous complete package verified on a Windows workstation |
| Windows ARM64 | Current cross-build | Not yet verified on target hardware |
| Linux x64 | Current cross-build | Earlier Ubuntu USB/tray operation checked; newest native sensor path needs validation |
| Linux ARM64 | Current cross-build | Not yet verified on target hardware |
| macOS Apple Silicon | Current cross-build | Menu bar, startup and SMC paths need a real Mac check |

Linux tray visibility depends on the desktop's StatusNotifier/AppIndicator support. macOS signing/Gatekeeper and enterprise restrictions still apply. CPU/GPU sensor support depends on the hardware and installed driver; unavailable values are shown as **--**.

## Report a useful compatibility result

Include the release version, OS/architecture, exact board model, CPU/GPU models, what worked, and the missing metric or error. Redact hostnames, usernames, serial numbers, tokens and process inventories. Never upload a whole-device flash dump: it can contain saved settings.
