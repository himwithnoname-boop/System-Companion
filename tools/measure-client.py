"""Windows resource sample, including existing helpers and all future child CPU.

Uses a job with no limits and no kill-on-close flag. Closing the measurement
does not stop or change the scheduling priority of the client.
"""
import argparse,ctypes,json,time
from ctypes import wintypes as w
from pathlib import Path

class Accounting(ctypes.Structure):
    _fields_=[('user',ctypes.c_int64),('kernel',ctypes.c_int64),('period_user',ctypes.c_int64),('period_kernel',ctypes.c_int64),('faults',w.DWORD),('processes',w.DWORD),('active',w.DWORD),('terminated',w.DWORD)]
class PIDs(ctypes.Structure):
    _fields_=[('assigned',w.DWORD),('count',w.DWORD),('ids',ctypes.c_size_t*64)]
class Memory(ctypes.Structure):
    _fields_=[('size',w.DWORD),('faults',w.DWORD)]+[(n,ctypes.c_size_t) for n in ['peak_rss','rss','peak_paged','paged','peak_nonpaged','nonpaged','pagefile','peak_pagefile','private']]

def measure(pids,seconds):
    import os
    k=ctypes.WinDLL('kernel32',use_last_error=True)
    k.CreateJobObjectW.argtypes=[ctypes.c_void_p,w.LPCWSTR];k.CreateJobObjectW.restype=w.HANDLE
    k.OpenProcess.argtypes=[w.DWORD,w.BOOL,w.DWORD];k.OpenProcess.restype=w.HANDLE
    k.AssignProcessToJobObject.argtypes=[w.HANDLE,w.HANDLE]
    k.QueryInformationJobObject.argtypes=[w.HANDLE,ctypes.c_int,ctypes.c_void_p,w.DWORD,ctypes.c_void_p]
    k.CloseHandle.argtypes=[w.HANDLE]
    k.K32GetProcessMemoryInfo.argtypes=[w.HANDLE,ctypes.c_void_p,w.DWORD]
    job=k.CreateJobObjectW(None,None)
    if not job:raise ctypes.WinError(ctypes.get_last_error())
    def accounting():
        value=Accounting()
        if not k.QueryInformationJobObject(job,1,ctypes.byref(value),ctypes.sizeof(value),None):raise ctypes.WinError(ctypes.get_last_error())
        return value
    try:
        for pid in pids:
            handle=k.OpenProcess(0x0100|0x0001,False,pid)
            if not handle:raise ctypes.WinError(ctypes.get_last_error())
            try:
                if not k.AssignProcessToJobObject(job,handle):raise ctypes.WinError(ctypes.get_last_error())
            finally:k.CloseHandle(handle)
        start=accounting();began=time.monotonic();rss=[];private=[];active=[]
        while time.monotonic()-began<seconds:
            ids=PIDs();k.QueryInformationJobObject(job,3,ctypes.byref(ids),ctypes.sizeof(ids),None)
            r=p=0
            for pid in ids.ids[:min(ids.count,64)]:
                handle=k.OpenProcess(0x1000|0x0010,False,pid)
                if handle:
                    value=Memory();value.size=ctypes.sizeof(value)
                    if k.K32GetProcessMemoryInfo(handle,ctypes.byref(value),value.size):r+=value.rss;p+=value.private
                    k.CloseHandle(handle)
            rss.append(r/1048576);private.append(p/1048576);active.append(ids.count)
            time.sleep(.2)
        ended=time.monotonic();end=accounting();cpu=(end.user+end.kernel-start.user-start.kernel)/1e7;wall=ended-began
        return {'seconds':round(wall,3),'cpu_seconds_including_children':round(cpu,6),'one_core_cpu_percent':round(100*cpu/wall,3),'machine_cpu_percent':round(100*cpu/wall/os.cpu_count(),4),'logical_processors':os.cpu_count(),'child_processes_started':end.processes-start.processes,'mean_working_set_MiB':round(sum(rss)/len(rss),2),'peak_sampled_working_set_MiB':round(max(rss),2),'mean_private_MiB':round(sum(private)/len(private),2),'max_active_processes':max(active),'initial_pids':pids}
    finally:k.CloseHandle(job)

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--pids',type=int,nargs='+',required=True);parser.add_argument('--seconds',type=int,default=30);parser.add_argument('--output',required=True);args=parser.parse_args()
    if not 10<=args.seconds<=60:parser.error('measurement window must be 10–60 seconds')
    result=measure(args.pids,args.seconds);Path(args.output).parent.mkdir(parents=True,exist_ok=True);Path(args.output).write_text(json.dumps(result,indent=2));print(json.dumps(result,indent=2))
