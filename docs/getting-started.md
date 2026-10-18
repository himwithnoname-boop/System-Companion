# Getting started

## What you need

- Waveshare **ESP32-S3-Touch-AMOLED-1.43-B**, 16 MB flash / 8 MB PSRAM.
- A USB data cable and a Windows x64 computer for the supplied installer.
- A Windows x64/ARM64, Linux x64/ARM64 or Apple Silicon Mac to monitor.
- A microSD card is optional. Networking is not needed for installation or ordinary monitoring.

## Flash a display

Download **SystemCompanion-Installer.exe** from [Releases](https://github.com/himwithnoname-boop/System-Companion/releases/latest). Copying just this EXE to a USB stick is sufficient. It includes its runtime, firmware and all OS clients.

Close any existing Monitor or serial terminal, finish card transfers and safely eject MoniStore. Run the EXE, select your matching board, and click **Install / Update**. Keep the USB cable connected until the installer reports verification and restart.

For an unrecognised blank board, hold **BOOT**, press and release **RESET**, then release BOOT. Click Refresh and retry. If more than one device is listed, select the correct one. The installer rejects the wrong ESP chip, incompatible flash capacity and protected programming configurations.

No whole-chip erase is used. Existing device settings and SD contents are preserved. Installing over vendor firmware replaces its application and client-storage area; keep a vendor backup if you need to return to it.

## Launch the host

| OS | Action |
| --- | --- |
| Windows | Open `Windows/Monitor.exe` on the client drive. A tray icon and dashboard appear. Hidden tray icons may be behind the taskbar arrow. |
| Linux | Run `Linux/Monitor`. If execution is restricted, open a terminal in that directory and run `sh ./Monitor`. Use your distribution's normal serial-port permissions. |
| macOS | Open `macOS/Monitor.app` on Apple Silicon. The app is unsigned; macOS security settings and filesystem execution rules may require explicit approval. |

The client selects its architecture without downloading a program. Its private extracted folder is removed on Exit. The Windows optional startup setting creates a stable per-user copy only after you enable it.

Closing the browser does not close the host. Reopen the dashboard from the tray/menu bar. Use **Exit System Companion** or **Exit monitor** to stop the host.

## First settings

Choose a theme, brightness, units and the GPUs to display, then **Apply to display**. The device confirms saved changes. Other open dashboards load the saved values on their next normal refresh. Closing a page does not discard settings already saved to the device; edits that were never applied are not saved.

Tap the centre for the next metric; swipe to change device pages. Read the [user guide](user-guide.md) for controls and [troubleshooting](troubleshooting.md) for missing sensors or ports.
