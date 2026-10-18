# User guide

## Display controls

- **Tap the home centre:** temperature → CPU load → GPU load → memory → download/upload → stacked CPU/GPU temperatures.
- **Horizontal swipe:** move through Overview, CPU, GPU, Memory, Storage, Network, Processes, AI and Settings.
- **Swipe down/up:** status alerts / quick actions.
- **Long press:** settings. **Double tap:** home.
- **GPU page:** up to four adapter summaries fit at once; tap to page through additional rows.
- **Storage:** vertical swipe selects a drive.
- **Chart time label:** select 60 seconds, 5 minutes or 30 minutes.
- **BOOT:** press to wake. Hold continuously for ten seconds, then release, to sleep. Release before ten seconds to cancel.
- **RESET:** immediate hardware reboot.

Auto rotation follows gravity at four right-angle orientations when enabled. It waits for stable orientation, pauses while touching, and retains its last orientation when lying flat.

## Dashboard settings and synchronization

The ESP32 is the source of saved display preferences. Each host reads them when connecting. Change dashboard controls and choose **Apply to display**; success is shown only after a matching device report. Physical setting changes and applied changes from another dashboard appear on the next active dashboard refresh.

Unapplied controls stay as local drafts rather than being overwritten by polling. Closing the dashboard leaves the host and device telemetry running. Reopen it from the tray/menu bar; the private session link is supplied afresh.

## GPU choices and bars

GPU0 is reserved for a detected integrated/onboard adapter. Discrete and unclassified adapters begin at GPU1 if no integrated GPU is identified. Choices use hardware identity rather than a GPU's current number.

Enable the GPUs you want in the dashboard and Apply. The home bar averages enabled adapters that expose a valid load:

| Enabled readings | Bar |
| --- | --- |
| GPU0 0%, GPU1 100% | 50% |
| Only GPU1 100% | 100% |
| GPU0 unavailable, GPU1 60% | 60% |
| All GPUs disabled, or no enabled load source | Empty / -- |

Cards and device rows show temperature, utilization, memory and available wattage. A driver's percentage power metric is never relabelled as watts. Full inventory collection continues so disabled GPUs can be selected again. Device preferences remember up to four customized computers and sixteen hidden GPU identities per profile. New GPUs start visible.

## Tray and Mac menu bar

**Open dashboard**, **Reconnect display**, **Run on startup (when display is connected)**, and **Exit** are common controls. Enabled startup has a checkmark. Linux needs a desktop with a StatusNotifier/AppIndicator host; dashboard controls remain available if the desktop does not provide one. The Mac client targets Apple Silicon and uses a native status-bar item.

Startup is optional, per computer/user, and disabled until selected. It runs at sign-in, checks for the display, and exits if absent. It does not install a system service or wait indefinitely for a later hotplug.

On Windows, optional CPU temperatures can require a separate explicit administrator approval. Remembered access and display preferences are different: access is saved on that Windows account, display settings live on the ESP32.

## MoniStore

An inserted card appears as an additional removable drive named **MoniStore** (FAT label **MONISTORE**). Internal client storage remains read-only and independent of the card. Cards are never automatically formatted. If the OS offers formatting, it will erase the card; use a card you intend to format and back up important data first. Safely eject before removal.

## Local AI page

Only activity on the monitored machine is considered. Remote ChatGPT/Codex or cloud API activity does not imply local inference load. Detection includes recognized Maestro, llama.cpp, Ollama, LM Studio, KoboldCPP, vLLM, SGLang, TGI and MLX LM sources when their processes or local APIs are accessible.

Active, queued, loaded, engine-running, idle and no-source states describe the evidence available. Token rates, queue depth and cache values require the relevant engine telemetry; unavailable fields stay --. VMs and containers may hide physical GPUs or engine endpoints.
