# USB passthrough in a virtual machine

Pass the ESP32's **composite USB device** through to the guest, not just a host folder. Close the host Monitor first. In VirtualBox, create a USB filter for vendor `303a` / application product `4002` and select your device; bootloader product IDs differ during updates.

Confirm in the guest with `lsusb` and the system journal. The application typically appears as a USB serial port plus a small read-only client drive and, if inserted, an SD drive. Port names and mount paths vary.

Launch `Linux/Monitor` from the guest-visible client drive, using `sh ./Monitor` if needed. Use the distribution's standard serial permissions. A supported desktop tray host supplies the icon; the dashboard provides controls even without it.

An ordinary VM reports guest CPU/RAM and may have no physical GPU or temperature sensors. Passing through this display does not pass through the host GPU. Only one OS can own the device's serial port at a time.
