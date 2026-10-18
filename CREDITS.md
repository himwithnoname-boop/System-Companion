# Credits

## Project

**himwithnoname-boop** — project direction, product requirements, hardware testing and maintenance. Original implementation developed with assistance from **OpenAI Codex**. The installer icon was generated with OpenAI image generation from an actual home-display reference; source artwork is retained in `tools/installer/assets`.

Actual device screenshots were captured over the project's USB framebuffer interface. The PayPal QR was supplied by the maintainer and is reproduced unchanged. PayPal and its logo are their owner's marks; inclusion does not imply endorsement.

## Hardware and firmware foundations

- [Waveshare](https://docs.waveshare.com/ESP32-S3-Touch-AMOLED-1.43/) — board design, pinout, controller documentation and demonstration references for the ESP32-S3-Touch-AMOLED-1.43-B. The earlier copied display-ID helper has been replaced; it is not redistributed.
- [Espressif ESP-IDF](https://github.com/espressif/esp-idf) and its component authors — processor SDK, USB, storage and LCD interfaces.
- [TinyUSB](https://github.com/hathach/tinyusb) — composite USB foundations.
- [LVGL](https://github.com/lvgl/lvgl) — display widgets and rendering.
- [Montserrat](https://github.com/JulietaUla/Montserrat) and [Font Awesome](https://fontawesome.com/) — embedded font/glyph data.

## Host and packaging foundations

- [gopsutil](https://github.com/shirou/gopsutil), [go.bug.st/serial](https://github.com/bugst/go-serial), [purego](https://github.com/ebitengine/purego), [godbus](https://github.com/godbus/dbus), Go's authors and the other pinned Go modules.
- [LibreHardwareMonitor](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor), [HIDSharp](https://software.seekye.com/hidsharp), [DiskInfoToolkit](https://github.com/Blacktempel/DiskInfoToolkit), [RAMSPDToolkit](https://github.com/Blacktempel/RAMSPDToolkit) and Microsoft runtime libraries.
- [esptool](https://github.com/espressif/esptool), [Python](https://www.python.org/), and their dependency authors — offline flashing.
- Igor Pavlov's [LZMA SDK](https://www.7-zip.org/sdk.html) — compression, native decoders and the Windows extractor.
- NVIDIA, Microsoft, the Linux kernel community and Apple — documented/native driver interfaces. Existing GPU drivers are queried; they are not redistributed or installed by this project.

Research references that informed interface investigation are listed in [sensor research](docs/sensor-research.md). GPL/AGPL or unlicensed research implementations were not copied into the original Go sensor providers.

All upstream rights remain with their owners. See [third-party notices](THIRD_PARTY_NOTICES.md) for terms and matching source.
