# Sensor coverage and research, client 1.0.7 / firmware 1.0.9

The product has three OS launchers and five native builds: Windows x64/ARM64, Linux x64/ARM64, and macOS Apple Silicon. Windows 10 and 11 share a client. Intel macOS is excluded by the requested target scope. The small Windows launcher selects the native architecture; both clients share one copy of the optional CPU helper libraries. All five clients fit the internal read-only USB volume; there is no runtime download.

## Implemented sources

| OS | Standard-user sources | Limits |
| --- | --- | --- |
| Windows | DXGI adapter identity; WDDM D3DKMT per-adapter temperature, fan RPM and memory clock; native PDH GPU engine utilization and dedicated memory; existing NVIDIA driver; existing LibreHardwareMonitor/OpenHardwareMonitor WMI CPU/GPU readings | WDDM/driver support varies. No universal standard-user CPU die API exists. Readable ACPI thermal zones are kept separate; generic TZ names are not relabelled as CPU. Compatible previously approved CPU helper remains optional. |
| Linux x64/ARM64 | CPU hwmon, explicitly CPU-named thermal zones, DRM adapter inventory, driver hwmon temperatures/power/fans and exposed load/memory/clocks; existing NVIDIA driver | Kernel sensor driver and file permissions determine coverage. ARM is an architecture, not a universal sensor interface. Generic SoC/ACPI thermal zones remain separately labelled. Virtual machines usually cannot see host sensors. |
| macOS Apple Silicon | Read-only AppleSMC temperature keys through IOKit, IOAccelerator GPU inventory/utilization/available driver temperatures | SMC keys and driver properties vary by hardware/macOS. No privileges are requested. This is an unsigned, non-sandboxed desktop app. An SMC GPU temperature is assigned only when one adapter exists; ambiguous readings are not attached to an arbitrary GPU. Physical Mac verification remains required. |

The optional Windows CPU helper uses the already installed compatible sensor driver. It does not install PawnIO, bypass enterprise policy or ask for approval during ordinary client launch. Sampling now runs independently of the read-only pipe server, so slow driver initialization cannot block a client request or repeatedly restart the reader. Driver exceptions retry on a bounded schedule. Replacing an already protected installed helper still requires an explicit user-approved setup; packaging alone does not update that protected copy.

## Supplied links

| Source | Useful findings and decision |
| --- | --- |
| [ExpertBeacon Python article](https://expertbeacon.com/check-the-temperature-of-your-cpu-using-python-and-other-cool-tricks/) | Page could not be fetched during research. Checked the underlying [psutil temperature documentation](https://psutil.readthedocs.io/en/latest/#psutil.sensors_temperatures): that API is platform limited and does not solve Windows CPU access. No dependency added. |
| [smctemp](https://github.com/narugit/smctemp) | AppleSMC confirms feasible standard-user Mac temperatures and key/data-format variation. GPL-2.0; no implementation copied. The provider independently reads the native protocol using the existing purego dependency. |
| [MacVitals](https://github.com/owieth/MacVitals) / [project site](https://owieth.github.io/MacVitals/) | SMC and IOAccelerator demonstrate useful native interfaces. No repository license found during inspection; no source copied. |
| [system-temp-monitor](https://github.com/AugustoPerboni/system-temp-monitor) | NVIDIA plus lm-sensors shell script with automatic sudo/package installation. Not integrated: it adds no universal temperature interface and its install behavior conflicts with portable, unprivileged launch. |
| [TempMon](https://github.com/infinityabundance/TempMon) | MIT Linux/Arch monitor; reinforces hwmon/lm-sensors paths. No new runtime or installation added. |
| [armtemp](https://github.com/spenceriam/armtemp) | AGPL-3.0 Windows Snapdragon example. Its sensor notes distinguish ACPI zones from CPU die readings. Independently implemented native PDH thermal-zone reads with honest provenance; no AGPL code copied. |
| [arm-cpu-temp](https://github.com/WendySanarwanto/arm-cpu-temp) | MIT, specific Linux ARM SBC paths. Generalized sysfs detection instead of assuming those paths apply to Apple or Windows ARM. |

## Primary interface references

- [Microsoft D3DKMT_ADAPTER_PERFDATA](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/d3dkmthk/ns-d3dkmthk-_d3dkmt_adapter_perfdata): temperature is tenths Celsius; power is a percentage, not watts. The latter is never shown as wattage.
- [Microsoft adapter address](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/d3dkmthk/ns-d3dkmthk-_d3dkmt_adapteraddress) and [adapter query kinds](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/d3dkmthk/ne-d3dkmthk-_kmtqueryadapterinfotype).
- [Microsoft DXGI enumeration](https://learn.microsoft.com/en-us/windows/win32/api/dxgi/nf-dxgi-idxgifactory1-enumadapters1) and [PDH wildcard arrays](https://learn.microsoft.com/en-us/windows/win32/api/pdh/nf-pdh-pdhgetformattedcounterarrayw).
- [Linux hwmon ABI](https://docs.kernel.org/hwmon/sysfs-interface.html), [thermal ABI](https://cdn.kernel.org/doc/html/latest/driver-api/thermal/sysfs-api.html), [AMDGPU thermal interfaces](https://kernel.org/doc/html/latest/gpu/amdgpu/thermal.html).

## Identity and display

GPU records retain stable provider identities and PCI/LUID matches where exposed. Identical model names do not collapse distinct cards. Missing values stay unavailable; a measured zero load remains zero. Summaries use the hottest reported temperature and average load across reporting adapters, while individual cards show their own metrics. Available VRAM/power totals sum only known values and may be partial. The round GPU page fits up to four adapters with smaller text and tap-to-page; the overview stacks temperatures or loads. More than eight adapters are sent in rotating bounded batches without discarding the dashboard inventory.

## Verification boundaries

On the local Windows x64 machine a non-elevated token read NVIDIA RTX PRO 6000 and AMD Radeon temperatures. A third fallback graphics adapter exposed no temperature. CPU readings continued through the existing remembered helper. Go fixtures cover missing/invalid readings, identical GPU identities, Linux CPU/ARM thermal provenance, SMC encodings and bounded wire pagination. The dashboard was checked with live data, 2/4/8/12-adapter layout fixtures, mobile width, safe text rendering and hidden-tab polling. Firmware compilation and cross-builds do not establish runtime compatibility on Windows ARM64, Linux ARM64 or Apple Silicon; those need target hardware checks. Firmware/device verification results are recorded separately in the release receipt.

The current activity bar averages enabled GPUs with available loads. Disabled adapters and unavailable loads are excluded. Device-saved GPU visibility profiles do not suppress collection, so adapters can be enabled again. The UI presents one summary per GPU instead of individual sensor rows.

GPU numbering reserves GPU0 for detected integrated graphics and starts discrete adapters at GPU1. With no identified integrated adapter, GPU0 is absent. Windows uses [DXCore IsIntegrated](https://learn.microsoft.com/en-us/windows/win32/api/dxcore_interface/ne-dxcore_interface-dxcoreadapterproperty), rather than enumeration order or video-memory size. Linux reads [AMD fusion flags](https://github.com/torvalds/linux/blob/master/include/uapi/drm/amdgpu_drm.h) and [Intel i915 memory regions](https://github.com/torvalds/linux/blob/master/include/uapi/drm/i915_drm.h)/[Xe configuration](https://github.com/torvalds/linux/blob/master/include/uapi/drm/xe_drm.h), when the existing device permissions permit the read-only query. Known SoC graphics drivers and Apple GPUs are integrated. Unknown types stay unclassified; they are not guessed to be integrated from their memory size or card number. Numbers do not replace hardware identity in saved selections.
