# Troubleshooting

## Installer cannot find or open the display

Use a USB data cable, connect one board, and choose Refresh. Exit Monitor and other serial applications. For a blank board hold BOOT, press/release RESET, then release BOOT. The target must be an ESP32-S3 with 16 MB flash and the supported board wiring.

The installer verifies chip/flash compatibility before writing. Protected flash configurations are rejected; there is no security bypass. If flashing verifies but USB does not reappear, release BOOT, press RESET once, and reconnect USB if necessary.

## Client drive missing

Check RESET and the USB data cable. The client filesystem is internal; no SD card is needed. An arbitrary ESP32 board will not become this monitor simply because it has the same chip.

On a virtual machine, pass through the composite USB device. The host OS and guest cannot own its serial interface at the same time. See [VM notes](ubuntu-vm.md).

## Linux serial permissions or missing tray

Use your distribution's normal serial-access policy. On Ubuntu this commonly means joining the appropriate serial group and signing out/in. Ask the administrator on managed systems.

Run `sh ./Monitor` from the device's Linux directory when the mount does not allow execution. The tray requires an existing StatusNotifier/AppIndicator host; the dashboard still offers Reconnect and Exit.

## Mac app blocked or no menu bar

Use an Apple Silicon Mac. The release is unsigned and not notarized; follow the Mac's normal explicit approval process, subject to organization policy. If execution from the USB filesystem is restricted, the packaged launcher can be run from Terminal with `sh`. Native menu/SMC operation is not yet physically verified on a Mac.

## Temperatures missing

Read [sensor coverage](sensors.md). Do not confuse no reading with 0°C. Existing drivers, permissions and hardware determine availability. A VM normally cannot see the host's physical sensor inventory. Windows CPU access may require the explicitly enabled helper; admin rights still cannot make an unsupported sensor work.

## GPU bar looks different from an individual GPU

The bar averages **enabled, reporting** adapters. Select only the desired GPU and Apply to display. Disabled adapters and missing loads are excluded. The GPU0 label is reserved for identified integrated graphics; discrete devices start at GPU1.

## Settings do not seem synchronized

Apply dashboard edits and wait for confirmation. The display must be connected. Other dashboards update during normal active polling; hidden pages resume polling when visible. Unapplied local drafts intentionally retain their current control value. Reopen the dashboard from the tray if its session link has expired.

## Security software or enterprise policy blocks an executable

The release is unsigned. Check its checksum and source; follow the computer owner's approval process. System Companion does not add antivirus exclusions or bypass policy. Do not assume every security detection is a confirmed false positive. Report reproducible details without uploading tokens, passwords, personal paths or full process dumps.

Use [Issues](https://github.com/himwithnoname-boop/System-Companion/issues) with OS/architecture, release versions, board model, GPU models and which fields are unavailable. Remove machine names and identifiers you do not want public.
