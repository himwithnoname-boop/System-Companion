"""Exercise LVGL's touch path and the live accelerometer on the connected board."""
import json,sys,time
from pathlib import Path
root=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(root/'tools'))
from device import Companion
d=Companion()
sample=json.loads((root/'.local/sample-current.json').read_text(encoding='utf-8-sig'))
try:
    d.settings(auto_rotate=False,timeout=0,page=0,center_mode=0)
    before=d.diagnostics()
    rotation=before['rotation']
    def point(x,y,state):
        if rotation==90:x,y=y,465-x
        elif rotation==180:x,y=465-x,465-y
        elif rotation==270:x,y=465-y,x
        d.send('touch',x=x,y=y,state=state)
        time.sleep(.08)
    def tap(x,y):
        point(x,y,1);point(x,y,0)
    tap(233,205);time.sleep(.4)  # Wake an idle display before testing metric taps.
    d.settings(center_mode=0)
    modes=[]
    for i in range(1,6):
        tap(233,205);time.sleep(.4)
        mode=d.diagnostics()['center_mode'];assert mode==i%5,(i,mode);modes.append(mode)
    # Swipes must change page without cycling the centre value.
    point(330,233,1);point(240,233,1);point(130,233,1);point(130,233,0);time.sleep(.4)
    diag=d.diagnostics();assert diag['page']==1 and diag['center_mode']==0,diag
    tap(233,205);time.sleep(.1);tap(233,205);time.sleep(.4)
    assert d.diagnostics()['page']==0
    point(233,205,1);time.sleep(.65);point(233,205,0);time.sleep(.4)
    assert d.diagnostics()['page']==8
    point(233,233,-1)
    d.settings(auto_rotate=True,page=0)
    time.sleep(1.5)
    diag=d.diagnostics();assert diag['imu'] and diag['auto_rotate']
    assert diag['uptime']>=before['uptime'],'Board reset during touch tests'
    sample['seq']=3000
    d.send('telemetry',**{k:v for k,v in sample.items() if k not in ('type','v')})
    time.sleep(.3)
    d.capture(root/'.local/qa/overview-final.png')
    d.settings(center_mode=4)
    time.sleep(.45)
    d.capture(root/'.local/qa/network-centre.png')
    d.settings(center_mode=0,page=0,timeout=120,auto_rotate=True)
    result={'checks':['Five centre taps cycled in the requested order','Swipe navigation preserved centre mode','Double tap returned home','Long press opened settings','Built-in accelerometer active; automatic rotation enabled'],'modes':modes,'diagnostics':diag}
    (root/'.local/qa/touch-results.json').write_text(json.dumps(result,indent=2))
    print(json.dumps(result,indent=2))
finally:
    d.send('touch',x=233,y=233,state=-1)
    d.close()
