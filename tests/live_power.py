"""Observe a physical BOOT cancel/off/wake sequence and RESET without changing settings."""
import json
from pathlib import Path
import sys
import time
import serial
from serial.tools import list_ports

root=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(root/'tools'))
from device import Companion

output=root/'.local/qa/live-power.json'
result={'shortHoldCancelled':False,'shutdownObserved':False,'deepSleepWakeObserved':False,
        'hardwareResetObserved':False,'preferencesPreserved':False,'orientations':{}}
device=None;initial=None;last_hold=0;hold_started=None;max_hold=0;gone_since=None
last_uptime=0;captured=False;candidate=None;candidate_since=0;started=time.monotonic()
fields=('theme','brightness','fahrenheit','auto_rotate','center_mode','timeout')
print('Watching BOOT power controls and RESET for five minutes.',flush=True)
try:
    while time.monotonic()-started<300:
        ports=[p.device for p in list_ports.comports() if p.vid==0x303a and p.pid==0x4002]
        if not ports:
            if device:
                device.close();device=None
            if gone_since is None:
                gone_since=time.monotonic()
                if max_hold>=8500:
                    result['shutdownObserved']=True
                    result['lastHoldMilliseconds']=max_hold
                    result['holdToUsbDisconnectSeconds']=round(gone_since-hold_started,3)
                    print('Verified: ten-second hold disconnected USB.',flush=True)
            time.sleep(.2);continue
        try:
            if device is None:
                device=Companion(ports[0]);gone_since=None
                assert device.hello['firmware']=='1.0.5',device.hello
                if initial is None:
                    initial=device.hello;result['before']=initial
            diag=device.diagnostics()
            now=time.monotonic();hold=diag['power_hold_ms']
            if hold and not last_hold:hold_started=now-hold/1000;max_hold=0
            max_hold=max(max_hold,hold)
            if not hold and 1500<=last_hold<8500 and not result['shutdownObserved']:
                result['shortHoldCancelled']=True
                print('Verified: early release cancelled shutdown.',flush=True)
            last_hold=hold
            if hold>=4000 and not captured:
                device.capture(root/'.local/qa/power-countdown.png');captured=True
            if result['shutdownObserved'] and diag['reset_reason']==8 and diag['wake_reason']==2:
                if not result['deepSleepWakeObserved']:
                    result['deepSleepWakeObserved']=True;result['wakeDiagnostics']=diag
                    print('Verified: BOOT woke the ESP32 from deep sleep.',flush=True)
            elif result['deepSleepWakeObserved'] and diag['wake_reason']==0 and diag['reset_reason']==1:
                result['hardwareResetObserved']=True;result['resetDiagnostics']=diag
                print('Verified: hardware RESET rebooted normally.',flush=True)
            x,y,z=diag['accel'];ax,ay=abs(x),abs(y)
            port=(('bottom' if x>0 else 'top') if ax>ay else ('right' if y>0 else 'left')) if abs(z)<=.8 and max(ax,ay)>=.65 and abs(ax-ay)>=.2 else None
            if port!=candidate:candidate=port;candidate_since=now
            if port and now-candidate_since>=1.8:
                expected={'bottom':270,'left':0,'top':90,'right':180}[port]
                assert diag['rotation']==expected,(port,expected,diag)
                result['orientations'][port]=diag
            result['lastDiagnostics']=diag;last_uptime=diag['uptime']
            output.write_text(json.dumps(result,indent=2),encoding='utf-8')
            if result['hardwareResetObserved']:
                device.send('hello');hello=device.receive('hello')
                result['after']=hello
                result['preferencesPreserved']=all(initial['settings'][k]==hello['settings'][k] for k in fields)
                break
        except (serial.SerialException,TimeoutError):
            if device:
                device.close();device=None
        time.sleep(.2)
finally:
    if device:device.close()
    output.write_text(json.dumps(result,indent=2),encoding='utf-8')
    print(json.dumps({k:v for k,v in result.items() if isinstance(v,(bool,int,float))},indent=2))
