# Supported hardware

The tested device is the **Waveshare ESP32-S3-Touch-AMOLED-1.43-B**, the case-equipped variant of Waveshare's 1.43-inch AMOLED development board.

| Item | Target |
| --- | --- |
| Processor | ESP32-S3 |
| Internal flash | 16 MB |
| PSRAM | 8 MB octal PSRAM |
| Display | 1.43-inch round AMOLED, 466 × 466, QSPI |
| Touch | FT3168-compatible I²C interface |
| Motion | QMI8658 accelerometer/gyroscope; accelerometer drives rotation |
| USB | Native USB; composite serial + read-only client drive + optional SD drive |
| microSD | Optional separate writable MoniStore volume |
| Buttons | BOOT for sleep/wake; RESET for hardware reboot |

## Official board information

- [Waveshare board documentation](https://docs.waveshare.com/ESP32-S3-Touch-AMOLED-1.43/)
- [Examples, schematic and controller datasheets](https://docs.waveshare.com/ESP32-S3-Touch-AMOLED-1.43/Resources-And-Documents)
- [Board family/product](https://www.waveshare.com/esp32-s3-touch-amoled-1.43.htm)
- [ESP32-S3 documentation](https://docs.espressif.com/projects/esp-idf/en/v5.5.5/esp32s3/)

The project uses this board's pinout and probes its display-ID register before selecting the initialization sequence. Panel revisions can differ. Similar-looking 1.43C/1.75-inch boards and other ESP32 devices are not supported by this image.

The compact display-ID implementation is project-owned MIT code. Waveshare's original helper is not redistributed. Board wiring, controller registers and timing follow the documented hardware interfaces.

## Flash layout

| Region | Offset | Size |
| --- | --- | --- |
| Bootloader | 0x000000 | Image-specific |
| Partition table | 0x008000 | 0x1000 allocation |
| NVS settings | 0x009000 | 0x6000 |
| PHY data | 0x00F000 | 0x1000 |
| Application | 0x010000 | 0x2F0000 |
| Client filesystem | 0x300000 | 0xD00000 |

Updates write the published image regions and preserve NVS. No SD card sectors are flashed.

## Power and removal

A ten-second BOOT hold enters deep sleep after release. It is not a physical battery disconnection; USB power indicators can remain lit. Eject MoniStore before shutdown or card removal. RESET reboots immediately. Holding BOOT during RESET enters programming mode.
