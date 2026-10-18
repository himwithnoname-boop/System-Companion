# Third-party notices

The root [MIT licence](LICENSE) covers original System Companion code and documentation. It does **not** replace upstream terms, grant trademark rights, or relicense bundled dependencies. Preserve this file and the matching licence texts when redistributing.

## Firmware and client libraries

| Component | Version / origin | Terms / source |
| --- | --- | --- |
| LVGL, vendored under firmware/components/lvgl | 8.3.11 | MIT; upstream file notices retained, [source](https://github.com/lvgl/lvgl/tree/v8.3.11), `licenses/LVGL-MIT.txt` |
| Montserrat font data | bundled with LVGL | SIL Open Font License 1.1, `licenses/Montserrat-OFL-1.1.txt` |
| Font Awesome glyph data | bundled with LVGL | OFL-1.1 for fonts; other upstream parts have CC-BY-4.0/MIT terms; `licenses/FontAwesome-5-LICENSE.txt` |
| ESP-IDF | 5.5.5 | Apache-2.0 and component-specific notices; [source](https://github.com/espressif/esp-idf/tree/v5.5.5), `licenses/ESP-IDF-Apache-2.0.txt` |
| esp_lcd_sh8601 | 1.0.0 | Apache-2.0, `licenses/SH8601-Apache-2.0.txt` |
| esp_tinyusb / TinyUSB / cmake_utilities | firmware/dependencies.lock | Upstream Apache-2.0/MIT notices retained; component-manager packages provide their licence files |
| gopsutil and other Go modules | host/go.mod and host/go.sum | Individual BSD/MIT/Apache notices in `licenses`; module versions and checksums pin source |
| LZMA SDK, static decoders and Windows SFX | 26.03 | Public-domain SDK source; `tools/vendor/lzma/NOTICE` and `licenses/LZMA-SDK.txt` |

Go module notices are stored under names such as `github.com_ebitengine_purego-LICENSE` and `go.bug.st_serial-LICENSE`. Test-only dependencies may also have retained notices. The Go toolchain's standard-library licence is included in release notices.

The project-owned LCD-ID helper is MIT code implementing a hardware register-read sequence. No licence is asserted over Waveshare's unlicensed original helper, which is excluded.

## Optional Windows CPU helper

| Redistributed component | Version / source identity | Licence |
| --- | --- | --- |
| LibreHardwareMonitorLib | 0.9.6, commit `3d331e3370efb858411f19511373eff65a218701` | MPL-2.0 |
| DiskInfoToolkit | 1.1.2, commit `25319eae5781e75bcf141e844ceab2afe94d40ea` | MPL-2.0 |
| RAMSPDToolkit-NDD | 1.4.2, commit `3b47b960e0830fef344624ad5e389675d5f0a1ce` | MPL-2.0 |
| HIDSharp | 2.6.4 | Apache-2.0, James F. Bellinger; `licenses/HidSharp-LICENSE.txt` |
| Microsoft.Bcl.AsyncInterfaces, System.IO.Pipelines | 10.0.3 | MIT and upstream notices |
| Microsoft.Bcl.HashCode | 6.0.0 | MIT and upstream notices |
| System.Buffers, System.Numerics.Vectors | 4.6.1 | MIT and upstream notices |
| System.Memory, System.Threading.Tasks.Extensions | 4.6.3 | MIT and upstream notices |
| System.Runtime.CompilerServices.Unsafe | 6.1.2 | MIT and upstream notices |

These libraries are unmodified. MPL-covered source archives for the listed commits are made available in the matching **SystemCompanion-ThirdPartySource.zip** release asset. The release source index includes URLs and SHA-256 hashes. MPL-2.0 text is in `licenses/LibreHardwareMonitor-MPL-2.0.txt` (identical terms also cover the two Toolkit libraries). Microsoft and HIDSharp notices are retained separately.

**PawnIO's kernel driver is not bundled or installed.** The optional helper uses a compatible existing installation only after explicit approval. Upstream LibreHardwareMonitor includes its own sensor/module resources and notices; its corresponding source archive is supplied without modifying those resources.

## Self-contained Windows flasher

The C# GUI is original MIT code. It starts a separate bundled Python worker. The esptool-based worker component is distributed in compliance with **GPL-2.0-or-later**; the MIT licence on project-owned source grants the permissions needed for this combined component. This does not turn every independent file in the repository into GPL code.

- **esptool 4.12.0** — GPL-2.0-or-later, Fredrik Ahlberg, Angus Gratton and Espressif contributors.
- Legacy flashing stub **v1.11.1** — GPL-2.0-or-later; [corresponding source](https://github.com/espressif/esptool-legacy-flasher-stub/tree/v1.11.1).
- New flashing stubs **v0.7.0** — MIT / Apache-2.0; [corresponding source](https://github.com/espressif/esp-flasher-stub/tree/v0.7.0).
- **CPython 3.12** — PSF and bundled dependency terms, retained in the embedded Python LICENSE file.
- **bitstring, cffi, pycparser, ecdsa, six, reedsolo, PyYAML** — respective upstream MIT licences/notices.
- **bitarray** — PSF-2.0.
- **pyserial and intelhex** — BSD-3-Clause.
- **cryptography** — Apache-2.0 / BSD-3-Clause with its retained third-party notices.

The EXE includes package source `.py` files, distribution metadata, licence texts and notices. The matching release additionally provides unmodified source distributions, flashing-stub sources and a source index. Sources and build instructions for esptool are therefore available alongside its binary distribution. No source archive is downloaded when an end user runs the installer.

## Source access and redistribution

Download the **ThirdPartySource** asset for the same release from [Releases](https://github.com/himwithnoname-boop/System-Companion/releases). `third-party/sources.json` records source origins and hashes. Original project source is supplied by the matching Git tag and automatic GitHub source archives.

Do not describe the complete installer bundle as “MIT only”. Keep dependency notices and corresponding-source availability with redistributed binaries. PayPal, Waveshare, Apple, Microsoft and NVIDIA names/logos retain their owners' rights.
