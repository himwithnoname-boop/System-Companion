# Build from source

End users should use the self-contained installer and on-device launchers. This guide is for developers. Tool downloads are build-time actions; installed clients do not download their dependencies.

## Toolchain

- Go 1.26 or newer (release build tested with 1.27.1).
- Python 3.12 on Windows for installer packaging, plus the pinned `tools/installer/requirements.txt`.
- ESP-IDF **5.5.5**, its Xtensa toolchain and 8 MB octal PSRAM / 16 MB flash support.
- An LLVM Clang cross-compiler supporting the static Linux decoder targets.
- LZMA SDK **26.03**: point `-LzmaSdk` at its extracted directory, containing `bin/x64/7zr.exe` and `bin/7zS2.sfx`.
- Windows .NET Framework C# compiler for the optional CPU helper and installer GUI.

## Host clients

From `host`:

```sh
go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o Monitor .
```

The Go code supports five targets: windows/amd64, windows/arm64, linux/amd64, linux/arm64 and darwin/arm64. Windows builds use the GUI subsystem; macOS uses native APIs through purego and does not require a Cgo cross-toolchain.

The complete Windows packaging script is:

```powershell
./tools/build-host.ps1 -Go go -Python python -Clang "path/to/clang.exe" -LzmaSdk "path/to/lzma-sdk"
```

`tools/prepare-dev-dependencies.py` optionally downloads checksum-pinned LibreHardwareMonitor and LZMA SDK build inputs into `.local`. It never installs a sensor driver. The CPU helper is built only when its library inputs are present. Go dependencies are obtained through the normal Go module mechanism.

Output: `dist/SYSTEM_MONITOR` (native builds) and `dist/DEVICE` (the complete space-bounded USB package). The latter must fit the existing storage partition; the packager checks allocation and reserve.

## Firmware

Use an exported ESP-IDF 5.5.5 shell after building `dist/DEVICE`:

```sh
idf.py -C firmware build
```

On Windows the wrapper also works from the exported environment:

```powershell
./tools/build-firmware.ps1 build
```

Its optional `-ToolchainRoot` supports a project-local ESP-IDF/tools layout using temporary drive mappings for long Windows paths. It is not required for the standard SDK path.

Images and offsets are generated in `firmware/build/flasher_args.json`. Never treat the application image as an offset-zero whole-flash image. Use the release installer or the generated manifest with esptool. Close Monitor first and discover the actual bootloader port; the application and bootloader can have different COM names.

## Windows installer

Create a Windows Python 3.12 environment and install:

```sh
python -m pip install -r tools/installer/requirements.txt
python tools/build-installer.py --flasher-python path/to/python.exe
```

The builder needs the completed firmware images and client filesystem. It embeds a private Python runtime, the esptool worker, validated flash images and the multi-size icon into a single Windows x64 GUI EXE. It verifies the isolated runtime before compiling. It is neither signed nor notarized.

## Tests

- `go test ./...` in `host` for parsers, GPU identity/selection, caches, settings and native structure contracts.
- `python tests/lcd_probe.py --cc clang --linker ld.lld` for the production display-ID protocol.
- `tests/orientation.py` and `tests/power_button.py` use the same native helper harness.
- `python tests/installer_worker.py` needs pyserial/esptool and tests corrupt images, layout, hardware/protection and port identity.
- Browser tests use Playwright and a local Chromium/Edge installation. Hardware/live tests require a connected device and private redirected session logs. Never publish those logs or tokens.

## Release

`tools/prepare-release-sources.py` gathers corresponding upstream sources and records hashes. `tools/package-release.py` builds only curated public assets and checksums; it excludes private QA, machine identifiers and logs. Follow [verification boundaries](verification.md) and retain all licences when redistributing.
