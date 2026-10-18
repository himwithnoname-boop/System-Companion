"""Test real BOOT-button debounce, ten-second hold and wake-press suppression."""
import ctypes
import json
from native_firmware import native_module

source='''#include "power_button.h"
static power_button_t button;
void reset_button(void) {button=(power_button_t){0};}
int tick_button(int down,long long now) {return power_button_update(&button,down,now);}
int remaining(long long now) {return power_button_remaining(&button,now);}
'''
with native_module(source,['reset_button','tick_button','remaining']) as module:
    tick=module.tick_button;tick.argtypes=[ctypes.c_int,ctypes.c_int64];tick.restype=ctypes.c_int
    remaining=module.remaining;remaining.argtypes=[ctypes.c_int64];remaining.restype=ctypes.c_int
    checks=0
    def check(down,now,expected=0):
        global checks
        result=tick(down,now)
        assert result==expected,(down,now,expected,result)
        checks+=1

    # A physical wake press held for more than ten seconds cannot shut down again.
    check(1,100000);check(1,140000);check(1,12000000)
    check(0,12010000);check(0,12050000)
    # A fresh press shuts down exactly once after its stable ten-second hold.
    check(1,13000000);check(1,13040000)
    assert remaining(14040000)==9
    check(1,23039999);check(1,23040000,2);check(1,24000000);check(0,25000000)

    module.reset_button();check(0,50000)
    # Short taps wake on release, and do not accumulate towards a long hold.
    for start in (100000,12000000,24000000):
        check(1,start);check(1,start+40000)
        check(0,start+100000);check(0,start+140000,1)
        assert remaining(start+200000)==-1
    # Contact bounce cannot produce a press or a release event.
    module.reset_button();check(0,50000)
    for down,now in [(1,100000),(0,110000),(1,120000),(0,130000),(0,170000)]:
        check(down,now)
    # Releasing early cancels; another hold gets a full ten seconds.
    check(1,200000);check(1,240000);check(1,10000000)
    check(0,10010000);check(0,10050000,1)
    check(1,11000000);check(1,11040000)
    check(1,21039999);check(1,21040000,2)
    print(json.dumps({'productionButtonChecks':checks,'tenSecondHold':True,
                      'wakePressIgnoredUntilRelease':True,'earlyReleaseCancels':True,
                      'debounced':True,'shutdownFiresOnce':True},indent=2))
