"""Verify the production LCD register-read sequence with all 256 response values."""
import ctypes,json
from native_firmware import native_module
source = r"""
#include "../components/read_lcd_id_bsp/lcd_probe.h"
static unsigned bits,command,samples,answer,millis,micros,resets,reset_levels,input_mode;
static void level(int pin,int value) {
 if(pin==PROBE_RESET) {resets++;reset_levels=(reset_levels<<1)|(unsigned)value;}
 if(pin==PROBE_DATA && bits<32) command=(command<<1)|(unsigned)value;
 if(pin==PROBE_CLOCK && value) bits++;
}
static void input(int enabled) {input_mode=enabled;}
static int sample(void) {if(!input_mode)return 0;return (answer>>(7-samples++))&1;}
static void us(uint32_t n) {micros+=n;}
static void ms(uint32_t n) {millis+=n;}
int exercise(unsigned value) {
 bits=command=samples=millis=micros=resets=reset_levels=input_mode=0;answer=value;
 lcd_probe_bus_t bus={level,input,sample,us,ms};
 int result=lcd_probe_identify(&bus);
 if(command!=0x0300da00u || bits!=40 || samples!=8 || millis!=360 || micros!=16 || resets!=3 || reset_levels!=5 || input_mode)return -1;
 return result;
}
"""
with native_module(source,["exercise"]) as module:
 module.exercise.argtypes=[ctypes.c_uint]
 for value in range(256):assert module.exercise(value)==value,value
print(json.dumps({"lcdProbeResponses":256,"commandAndResetTimingVerified":True}))
