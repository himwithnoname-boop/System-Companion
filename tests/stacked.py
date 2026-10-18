"""Hardware rendering fixtures for the sixth overview mode; close Monitor first."""
import json,sys,time
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]/'tools'))
from device import Companion

root=Path(__file__).resolve().parents[1]
real=json.loads((root/'.local/live-sample.json').read_text(encoding='utf-8-sig'))
device=Companion();result={'firmware':device.hello['firmware'],'touchModes':[],'captures':'controlled temperature fixtures'}
try:
    assert device.hello['firmware']=='1.0.2'
    before=device.diagnostics();result['preservedAutoRotate']=before['auto_rotate'];result['preservedBrightness']=before['brightness']
    device.settings(center_mode=5,page=0,fahrenheit=False)
    fixture=dict(real,type='telemetry',v=1,seq=8000,cpu_temp=50,gpu_temp=40)
    device.send('telemetry',**{k:v for k,v in fixture.items() if k not in ('type','v')});time.sleep(.2)
    device.capture(root/'.local/qa/stacked-celsius.png')
    device.settings(fahrenheit=True)
    device.send('telemetry',**{k:v for k,v in fixture.items() if k not in ('type','v')});time.sleep(.2)
    device.capture(root/'.local/qa/stacked-fahrenheit.png')
    device.settings(fahrenheit=False)
    fixture['cpu_temp']=None
    device.send('telemetry',**{k:v for k,v in fixture.items() if k not in ('type','v')});time.sleep(.2)
    device.capture(root/'.local/qa/stacked-unavailable.png')
    device.settings(center_mode=0)
    for mode in [1,2,3,4,5,0]:
        device.send('touch',x=233,y=233,state=1);time.sleep(.1)
        device.send('touch',x=233,y=233,state=0);time.sleep(.45)
        actual=device.diagnostics()['center_mode'];result['touchModes'].append(actual);assert actual==mode,(actual,mode)
finally:
    device.send('touch',x=233,y=233,state=-1)
    device.settings(center_mode=5,page=0,fahrenheit=False)
    device.send('telemetry',**{k:v for k,v in real.items() if k not in ('type','v')})
    device.close()
(root/'.local/qa/stacked-results.json').write_text(json.dumps(result,indent=2))
print(json.dumps(result,indent=2))
