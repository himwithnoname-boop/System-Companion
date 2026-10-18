"""Companion diagnostics and display capture. Requires pyserial (ESP-IDF Python provides it)."""
import argparse
import base64
import json
from pathlib import Path
import struct
import time
import zlib
import serial
from serial.tools import list_ports

class Companion:
    def __init__(self, port=None):
        ports = [port] if port else [p.device for p in list_ports.comports() if p.vid == 0x303A and p.pid == 0x4002]
        for candidate in ports:
            self.serial = serial.Serial(candidate, 115200, timeout=0.5)
            self.serial.reset_input_buffer()
            self.send('hello')
            try:
                self.hello = self.receive('hello', 3)
                if self.hello.get('device') != 'system-companion':
                    raise RuntimeError('Wrong device')
                self.port = candidate
                return
            except (RuntimeError, TimeoutError):
                self.serial.close()
        raise RuntimeError('System Companion USB device not found; close Monitor before diagnostics.')

    def send(self, kind, **fields):
        self.serial.write((json.dumps(dict(type=kind, v=1, **fields), separators=(',', ':'))+'\n').encode())

    def receive(self, kind, timeout=5):
        end = time.monotonic()+timeout
        while time.monotonic() < end:
            line = self.serial.readline()
            if not line:
                continue
            try:
                msg = json.loads(line)
            except (ValueError, UnicodeDecodeError):
                continue
            if msg.get('type') == kind:
                return msg
        raise TimeoutError(f'Timed out waiting for {kind}')

    def diagnostics(self):
        self.send('diagnostics')
        return self.receive('diagnostics')

    def settings(self, **fields):
        self.send('settings', **fields)
        return self.receive('ack')

    def capture(self, path):
        self.send('capture')
        start = self.receive('frame_begin', 5)
        raw = bytearray(start['bytes'])
        received = 0
        while received < len(raw):
            msg = self.receive('frame_data', 10)
            part = base64.b64decode(msg['data'], validate=True)
            if msg['offset'] != received:
                raise RuntimeError('Display frame has a missing chunk')
            raw[received:received+len(part)] = part
            received += len(part)
        self.receive('frame_end', 5)
        w,h = start['width'],start['height']
        if len(raw) != w*h*2:
            raise RuntimeError('Unexpected snapshot color format')
        rows = bytearray()
        for y in range(h):
            rows.append(0)
            for x in range(w):
                i = 2*(y*w+x)
                pixel = raw[i]*256+raw[i+1]
                rows.extend((((pixel>>11)&31)*255//31,((pixel>>5)&63)*255//63,(pixel&31)*255//31))
        def chunk(kind, data):
            return struct.pack('>I',len(data))+kind+data+struct.pack('>I',zlib.crc32(kind+data)&0xffffffff)
        png=b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR',struct.pack('>IIBBBBB',w,h,8,2,0,0,0))+chunk(b'IDAT',zlib.compress(rows))+chunk(b'IEND',b'')
        Path(path).parent.mkdir(parents=True,exist_ok=True)
        Path(path).write_bytes(png)

    def close(self):
        self.serial.close()

def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--port')
    parser.add_argument('--page',type=int,choices=range(9))
    parser.add_argument('--capture')
    parser.add_argument('--sample')
    parser.add_argument('--bootloader',action='store_true')
    args=parser.parse_args()
    device=Companion(args.port)
    try:
        print(json.dumps(device.hello))
        if args.sample:
            sample=json.loads(Path(args.sample).read_text(encoding='utf-8-sig'))
            device.send('telemetry',**{k:v for k,v in sample.items() if k not in ('type','v')})
        if args.page is not None:
            device.settings(page=args.page)
            time.sleep(.5)
        if args.capture:
            device.capture(args.capture)
            print('Saved',args.capture)
        if args.bootloader:
            device.send('bootloader')
            device.receive('ack')
        else:
            print(json.dumps(device.diagnostics()))
    finally:
        device.close()

if __name__=='__main__':
    main()
