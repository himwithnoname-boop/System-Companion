"""Live fault check after terminating only the monitor's NVIDIA child.

Usage: python tests/recovery.py <redirected-client-log>
The monitor remains running; this test waits for automatic GPU recovery.
"""
import json,re,sys,time,urllib.request
from pathlib import Path

match=re.search(r'(http://127\.0\.0\.1:8765)/#([a-f0-9]{48})',Path(sys.argv[1]).read_text())
assert match,'Agent URL missing'
headers={'Authorization':'Bearer '+match[2]}
def status():
    with urllib.request.urlopen(urllib.request.Request(match[1]+'/api/status',headers=headers),timeout=3) as r:
        return json.load(r)

start=time.monotonic();samples=[];expired_at=None;recovered_at=None;previous=None
while time.monotonic()-start<50:
    s=status();t=s['telemetry'];age=time.monotonic()-start
    assert s['port'] and not s['error'],s
    for key in ['cpu_load','ram_used','net_down']:assert isinstance(t.get(key),(int,float)),key
    if t['seq']!=previous:
        samples.append((t['seq'],age));previous=t['seq']
    if t.get('gpu_temp') is None:
        if expired_at is None:expired_at=age
    elif expired_at is not None:
        recovered_at=age;break
    time.sleep(.2)
assert expired_at is not None,'Stopped GPU reader left its old temperature visible'
assert expired_at<6,'GPU cache did not expire promptly'
assert recovered_at is not None,'GPU reader did not restart automatically'
gaps=[b[1]-a[1] for a,b in zip(samples,samples[1:])]
assert max(gaps)<1.5,('CPU/USB monitoring stalled',max(gaps))
result={'gpuCacheExpiredAfterSeconds':round(expired_at,2),'gpuRecoveredAfterSeconds':round(recovered_at,2),'uniqueSamplesDuringFailure':len(samples),'maximumSampleGapSeconds':round(max(gaps),3),'otherMetricsRemainedAvailable':True}
Path('.local/qa/recovery-optimized.json').write_text(json.dumps(result,indent=2))
print(json.dumps(result,indent=2))
