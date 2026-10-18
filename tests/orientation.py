"""Compile and exercise the actual firmware orientation helper on this host.

Usage: python tests/orientation.py --cc clang [--linker ld.lld]
Windows can use the bundled NDK LLVM compiler without installing a C runtime.
"""
import ctypes
import json
import math
from native_firmware import native_module

with native_module('#include "orientation.h"\nint sensor_rotation(float x,float y,float z) {'
                   'return orientation_target(x,y,z);}\n', ['sensor_rotation']) as module:
    target = module.sensor_rotation
    target.argtypes = [ctypes.c_float]*3
    target.restype = ctypes.c_int
    checks = 0

    def check(sample, expected):
        global checks
        actual = target(*sample)
        assert actual==expected, (sample, expected, actual)
        checks += 1

    # Physical references established from the live port-bottom and face-up readings.
    cases = [('bottom', (1,0,0), 3), ('left', (0,-1,0), 0),
             ('top', (-1,0,0), 1), ('right', (0,1,0), 2)]
    for port, axis, expected in cases:
        check(axis, expected)
        for tilt in (-.35, 0, .35):
            for noise in (-.08, 0, .08):
                x,y,_ = axis
                check((x*.95 + (noise if not x else 0),
                       y*.95 + (noise if not y else 0), tilt), expected)

    # Turning the board clockwise moves the port bottom -> left -> top -> right.
    # Verify its rendered top points towards gravity's upward vector in raw pixels.
    raw_top = [(0,-1), (-1,0), (0,1), (1,0)]
    for degrees in range(0,360,10):
        angle = math.radians(degrees)
        x,y = math.cos(angle), -math.sin(angle)
        rotation = target(x,y,0)
        if abs(abs(x)-abs(y))<.2:
            check((x,y,0), -1)
        else:
            assert rotation>=0, (degrees, rotation)
            tx,ty = raw_top[rotation]
            assert tx*x+ty*y>.707, (degrees, rotation, 'rendered top faces sideways/down')
            checks += 1

    for sample in [(0,0,-1), (.03,-.03,-1.02), (0,0,1), (.8,0,.9),
                   (0,0,0), (.5,0,0), (.707,.707,0), (-.707,.707,0),
                   (.707,-.707,0), (-.707,-.707,0)]:
        check(sample, -1)
    for value in (math.nan, math.inf, -math.inf):
        for axis in range(3):
            sample = [1,0,0]
            sample[axis] = value
            check(sample, -1)

    result = {'productionMappingChecks':checks, 'portsClockwise':['bottom','left','top','right'],
              'lvglDegrees':[270,0,90,180], 'flatAndDiagonalHold':True,
              'invalidReadingsHold':True}
    print(json.dumps(result, indent=2))
