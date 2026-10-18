"""Observe all four physical orientations and flat hold without changing settings.

Close Monitor, then turn the display slowly through each side (pause two seconds)
and lay it flat on its back. Run: python tests/live_rotation.py
"""
import argparse
import json
from pathlib import Path
import sys
import time

root=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(root/'tools'))
from device import Companion

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--duration',type=float,default=180)
parser.add_argument('--output',type=Path,default=root/'.local/qa/live-rotation.json')
args=parser.parse_args()
d=Companion()
seen={};flat=None;candidate=None;candidate_since=0;last_rotation=None
started=time.monotonic()
try:
    initial=d.hello
    assert initial['settings']['auto_rotate'], 'Enable automatic rotation before testing'
    print('Observing physical rotations; hold each side for two seconds, then lay flat.',flush=True)
    while time.monotonic()-started<args.duration:
        diag=d.diagnostics()
        assert diag['imu'], 'Accelerometer is unavailable'
        x,y,z=diag['accel'];ax,ay,az=abs(x),abs(y),abs(z)
        key=None
        if az<=.8 and max(ax,ay)>=.65 and abs(ax-ay)>=.2:
            key=('bottom' if x>0 else 'top') if ax>ay else ('right' if y>0 else 'left')
        elif az>.9 and max(ax,ay)<.15:
            key='flat'
        now=time.monotonic()
        if key!=candidate:
            candidate=key;candidate_since=now;last_rotation=diag['rotation']
        if key and now-candidate_since>=1.8:
            if key=='flat':
                assert diag['rotation']==last_rotation, 'Rotation changed while lying flat'
                if len(seen)==4 and flat is None:
                    flat=diag;print('Verified: flat position retains its orientation.',flush=True)
            else:
                expected={'bottom':270,'left':0,'top':90,'right':180}[key]
                assert diag['rotation']==expected,(key,expected,diag)
                if key not in seen:
                    seen[key]=diag;print(f'Verified: port {key}, LVGL rotation {expected}.',flush=True)
        if len(seen)==4 and flat is not None:
            break
        time.sleep(.2)
    d.send('hello');final=d.receive('hello')
    result={'firmware':initial['firmware'],'physicalOrientations':seen,'flatHold':flat,
            'allFourObserved':len(seen)==4,'preferencesPreserved':initial['settings']==final['settings'],
            'completed':len(seen)==4 and flat is not None}
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(result,indent=2),encoding='utf-8')
    print(json.dumps({'allFourObserved':result['allFourObserved'],'flatHold':flat is not None,
                      'preferencesPreserved':result['preferencesPreserved'],'output':str(args.output)},indent=2))
    if not result['completed']:
        raise SystemExit('Physical check incomplete: '+', '.join(sorted(set(['bottom','left','top','right'])-set(seen))))
finally:
    d.close()
