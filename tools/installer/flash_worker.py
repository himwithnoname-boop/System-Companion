"""Offline flasher worker. JSON lines are consumed by the Windows GUI."""
import argparse, contextlib, hashlib, io, json, re, sys, time
from pathlib import Path
import serial
from serial.tools import list_ports
import esptool
from esptool.targets.esp32s3 import ESP32S3ROM
from esptool.cmds import DETECTED_FLASH_SIZES

ROOT = Path(__file__).resolve().parent
APP = (0x303A, 0x4002)
BOOTS = {(0x303A, 0x0009), (0x303A, 0x1001)}
USB_BRIDGES = {(0x10C4, 0xEA60), (0x1A86, 0x7523), (0x1A86, 0x55D4)}
OUT = sys.stdout

def emit(kind, **data):
    print(json.dumps(dict(event=kind, **data)), file=OUT, flush=True)

class LogWriter(io.TextIOBase):
    def __init__(self): self.buffer = ""
    def write(self, text):
        self.buffer += text
        while "\n" in self.buffer or "\r" in self.buffer:
            line, self.buffer = re.split(r"[\r\n]", self.buffer, maxsplit=1)
            if line.strip():
                match = re.search(r"\((\d+) %\)", line)
                emit("log", text=line.strip(), percent=int(match[1]) if match else None)
        return len(text)
    def flush(self):
        if self.buffer.strip(): emit("log", text=self.buffer.strip())
        self.buffer = ""

def normal_serial(value): return re.sub("[^0-9a-z]", "", (value or "").lower())

def devices():
    return [dict(port=p.device, serial=p.serial_number or "", description=p.description,
                 vid=p.vid, pid=p.pid, companion=(p.vid,p.pid)==APP)
            for p in sorted(list_ports.comports(), key=lambda p:p.device)
            if (p.vid,p.pid) in ({APP}|BOOTS|USB_BRIDGES)]

def verify_bundle(root=ROOT):
    manifest = json.loads((root/"image.json").read_text(encoding="utf-8"))
    if manifest["chip"] != "esp32s3" or manifest["flash_bytes"] != 16*1024*1024:
        raise ValueError("Unsupported target manifest")
    expected = {0:0x8000, 0x8000:0x1000, 0x10000:0x2f0000, 0x300000:0xd00000}
    seen=set()
    for item in manifest["images"]:
        offset=int(item["offset"],0)
        path=root/item["file"]
        if offset not in expected or offset in seen or path.parent != root:
            raise ValueError("Invalid flash layout")
        seen.add(offset)
        data=path.read_bytes()
        if len(data)!=item["bytes"] or len(data)>expected[offset] or hashlib.sha256(data).hexdigest()!=item["sha256"]:
            raise ValueError("Damaged firmware payload: "+item["file"])
    if seen != set(expected): raise ValueError("Incomplete firmware payload")
    return manifest

def get_port(name, expected_serial):
    candidates=[p for p in list_ports.comports() if p.device==name]
    if len(candidates)!=1: raise RuntimeError("The selected device disconnected. Refresh and try again.")
    p=candidates[0]
    if expected_serial and normal_serial(p.serial_number)!=normal_serial(expected_serial):
        raise RuntimeError("A different device is now using that port. Refresh and select it again.")
    if (p.vid,p.pid) not in ({APP}|BOOTS|USB_BRIDGES):
        raise RuntimeError("The port is not a supported Espressif USB or serial bridge.")
    return p

def enter_bootloader(port):
    if (port.vid,port.pid)==(0x303A,0x0009): return port.device, "no_reset"
    if (port.vid,port.pid)!=APP: return port.device, "default_reset"
    with serial.Serial(port.device,115200,timeout=.5,write_timeout=2) as link:
        link.write(b'{"type":"hello","v":1}\n')
        deadline=time.monotonic()+4
        while time.monotonic()<deadline:
            try: msg=json.loads(link.readline())
            except (ValueError,UnicodeDecodeError): continue
            if msg.get("type")=="hello" and msg.get("device")=="system-companion": break
        else: raise RuntimeError("The selected device did not identify as System Companion.")
        link.write(b'{"type":"bootloader","v":1}\n')
        link.flush()
    identity=normal_serial(port.serial_number)
    for _ in range(80):
        candidates=[p for p in list_ports.comports() if (p.vid,p.pid) in BOOTS and
                    identity and normal_serial(p.serial_number)==identity]
        if len(candidates)==1: return candidates[0].device, "no_reset"
        time.sleep(.1)
    raise RuntimeError("Bootloader not found. Hold BOOT, press RESET, release BOOT, then Refresh.")

def confirm_target(esp):
    if esp.CHIP_NAME != "ESP32-S3": raise RuntimeError("This installer requires ESP32-S3; no data was written.")
    if esp.secure_download_mode or esp.get_secure_boot_enabled() or esp.get_flash_encryption_enabled() or esp.stub_is_disabled:
        raise RuntimeError("This device has protected flash or restricted programming. No data was written.")
    esp.flash_spi_attach(0)  # ROM does not attach the flash interface itself.
    flash_id=esp.flash_id()
    size=DETECTED_FLASH_SIZES.get((flash_id >> 16) & 0xff)
    if size is None:
        raise RuntimeError("The flash chip did not identify correctly (ID %06x). No data was written." % flash_id)
    if size != "16MB":
        raise RuntimeError("This build requires 16 MB flash; detected %s. No data was written." % size)

def resume(esp):
    # Native USB's forced-download RTC bit can survive a normal USB reset.
    esp.write_reg(ESP32S3ROM.RTC_CNTL_OPTION1_REG,0,ESP32S3ROM.RTC_CNTL_FORCE_DOWNLOAD_BOOT_MASK)
    try: esp.watchdog_reset()
    except serial.SerialException: pass

def wait_for_app(identity, timeout=20):
    end=time.monotonic()+timeout
    while time.monotonic()<end:
        ports=[p for p in list_ports.comports() if (p.vid,p.pid)==APP and
               identity and normal_serial(p.serial_number)==identity]
        if len(ports)==1:
            try:
                with serial.Serial(ports[0].device,115200,timeout=.5,write_timeout=2) as link:
                    link.write(b'{"type":"hello","v":1}\n')
                    for _ in range(6):
                        try: msg=json.loads(link.readline())
                        except (ValueError,UnicodeDecodeError): continue
                        if msg.get("type")=="hello" and msg.get("device")=="system-companion": return msg
            except serial.SerialException: pass
        time.sleep(.25)
    return None

def flash(port_name, expected_serial):
    manifest=verify_bundle()
    port=get_port(port_name, expected_serial)
    identity=normal_serial(port.serial_number)
    emit("stage",text="Connecting to the ESP32 bootloader…")
    boot,mode=enter_bootloader(port)
    esp=None
    try:
        esp=esptool.detect_chip(boot,115200,connect_mode=mode,connect_attempts=3)
        confirm_target(esp)
        identity=normal_serial(":".join("%02x"%b for b in esp.read_mac()))
        # Keep the checked connection open. No other device can replace it.
        emit("stage",text="Installing firmware and all five offline clients. Keep USB connected.")
        images=manifest["images"]
        args=["--chip","esp32s3","--port",boot,"--baud","921600","--after","no_reset_stub","write_flash",
              "--flash_mode","dio","--flash_freq","80m","--flash_size","16MB"]
        for image in images: args += [image["offset"],str(ROOT/image["file"])]
        # esptool verifies each written region against the target flash digest.
        # No erase-all: the NVS preferences and MoniStore SD are untouched.
        esptool.main(args,esp=esp)
        emit("stage",text="Flash hashes verified. Restarting and checking the display…")
        esp=esp.STUB_CLASS(esp)  # main leaves the official stub running
        resume(esp)
    finally:
        if esp is not None: esp._port.close()
    hello=wait_for_app(identity)
    if hello:
        emit("complete",verified=True,text="Installed and verified. The display and USB client drive are ready.",
             firmware=hello.get("firmware",hello.get("version")),client=manifest["client_version"])
    else:
        emit("complete",verified=False,text="Flash verified. Press RESET once without BOOT, then reconnect USB if the client drive has not appeared.",
             client=manifest["client_version"])

def main():
    parser=argparse.ArgumentParser()
    parser.add_argument("--list",action="store_true")
    parser.add_argument("--verify",action="store_true")
    parser.add_argument("--flash")
    parser.add_argument("--serial",default="")
    args=parser.parse_args()
    try:
        manifest=verify_bundle()
        if args.list: emit("devices",devices=devices(),version=manifest["client_version"])
        elif args.verify: emit("verified",manifest=manifest,esptool=esptool.__version__)
        elif args.flash:
            writer=LogWriter()
            try:
                with contextlib.redirect_stdout(writer): flash(args.flash,args.serial)
            finally: writer.flush()
        else: parser.error("an action is required")
    except Exception as error:
        emit("error",text=str(error)+"\nIf the port is busy, exit Monitor from its tray menu. For a blank device, hold BOOT, press RESET, then release BOOT and retry.")
        return 1
    return 0
if __name__=="__main__": sys.exit(main())
