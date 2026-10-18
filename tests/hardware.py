"""Run only against a Companion; close the agent first. Uses a real host sample."""
import json
from pathlib import Path
import sys
import time
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'tools'))
from device import Companion

root=Path(__file__).resolve().parents[1]
sample=json.loads((root/'.local/live-sample.json').read_text(encoding='utf-8-sig'))
device=Companion()
results={'hello':device.hello,'checks':[]}
try:
    device.settings(timeout=0,theme=0,brightness=170,fahrenheit=False,page=0)
    before=device.diagnostics()
    # Fragmentation, oversize input, invalid JSON and a future protocol version.
    good=dict(sample,v=1,type='telemetry',seq=2000)
    wire=(json.dumps(good,separators=(',',':'))+'\n').encode()
    assert len(wire)<8192
    for offset in range(0,len(wire),137):
        device.serial.write(wire[offset:offset+137])
    time.sleep(.2)
    assert device.diagnostics()['seq']==2000
    device.serial.write(b'x'*9000+b'\n{bad json}\n'+b'{"type":"telemetry","v":2,"seq":999999}\n')
    time.sleep(.2)
    after=device.diagnostics()
    assert after['seq']==2000 and after['rejected']>=before['rejected']+3
    results['checks'].append('Fragmented frames accepted; malformed, oversized and future-version frames rejected')
    for page in range(9):
        sample['seq']=2001+page
        device.send('telemetry',**{k:v for k,v in sample.items() if k not in ('type','v')})
        device.settings(page=page)
        time.sleep(.45)
        assert device.diagnostics()['page']==page
        device.capture(root/f'.local/qa/page-{page}.png')
    results['checks'].append('All nine pages rendered and captured on the ESP32')
    # Every built-in theme must rebuild safely with the same real sample.
    for theme in range(5):
        device.settings(page=0,theme=theme)
        assert device.diagnostics()['theme']==theme
    results['checks'].append('All five themes applied')
    device.settings(page=0,theme=0,timeout=120)
    results['diagnostics']=device.diagnostics()
    (root/'.local/qa/hardware-results.json').write_text(json.dumps(results,indent=2))
    print(json.dumps(results,indent=2))
finally:
    device.close()
