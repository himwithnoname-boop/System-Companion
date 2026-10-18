"""Resume flash boot after esptool resets the native USB Serial/JTAG interface."""
import time
from serial import SerialException
from serial.tools import list_ports
from esptool.targets.esp32s3 import ESP32S3ROM

def ports(product):
    return [p.device for p in list_ports.comports() if (p.vid,p.pid)==(0x303a,product)]

boot_ports=ports(0x1001)+ports(0x0009)

if ports(0x4002):
    print('Companion firmware is running')
elif boot_ports:
    esp=ESP32S3ROM(boot_ports[0],115200)
    esp.connect(mode='no_reset')
    esp.write_reg(esp.RTC_CNTL_OPTION1_REG,0,esp.RTC_CNTL_FORCE_DOWNLOAD_BOOT_MASK)
    try:
        esp.watchdog_reset()
    except SerialException:
        pass  # USB disappears immediately when the watchdog fires.
    finally:
        esp._port.close()
    for _ in range(50):
        if ports(0x4002):
            print('Companion firmware resumed successfully')
            break
        time.sleep(.2)
    else:
        raise SystemExit('Firmware did not enumerate. Release BOOT and press RESET.')
else:
    raise SystemExit('No connected ESP32 found')
