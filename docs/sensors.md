# Sensors and permissions

| Platform | Standard-user sources | Practical limits |
| --- | --- | --- |
| Windows | DXGI/DXCore inventory; D3DKMT temperature/fan/memory-clock queries; PDH GPU load; existing NVIDIA driver; permitted existing CPU providers | Driver exposure varies. Windows has no universal standard-user CPU die-temperature API. Generic ACPI thermal zones are labelled separately. |
| Linux | CPU hwmon; explicitly CPU-labelled thermal zones; DRM GPU inventory and exposed hwmon/load/VRAM/power; existing NVIDIA driver | Kernel drivers and device-file permissions determine coverage. ARM is not one universal sensor interface. |
| macOS Apple Silicon | Read-only AppleSMC and IOAccelerator properties via native APIs | Sensor keys vary by Mac/macOS; physical Mac verification is pending. No automatic elevation or kernel driver is used. |

A measured zero utilization is valid. A missing temperature is **--**. Stale values expire instead of remaining falsely live. GPUs are matched by stable native identity/PCI/LUID where available; identical model names are kept distinct.

## Optional Windows CPU temperatures

The default host remains a standard-user application. If native/permitted sources cannot provide CPU temperature, the **CPU temperatures** menu/dashboard option can request one administrator-approved setup. It uses an existing compatible PawnIO installation and never downloads or installs a driver automatically.

The approved reader is stored under protected Program Files, uses an on-demand task and a restricted read-only local pipe, and stops after inactivity or normal host Exit. Turning the option off stops the reader. Later approved launches reuse access without another UAC prompt. Enterprise security policy may prohibit this path. CPU load, RAM, network and available GPU telemetry remain usable.

A driver installation or policy bypass is not a requirement silently performed by System Companion. GPU temperature access is separate from the optional CPU setup.

[Research notes and primary interface references](sensor-research.md).
