"""Load a small production C helper for host tests without an installed C runtime."""
import argparse
from contextlib import contextmanager
import ctypes
from pathlib import Path
import subprocess
import sys
import tempfile

root=Path(__file__).resolve().parents[1]

@contextmanager
def native_module(source_text,exports):
    parser=argparse.ArgumentParser()
    parser.add_argument('--cc',default='cc')
    parser.add_argument('--linker',default='ld.lld')
    args=parser.parse_args()
    with tempfile.TemporaryDirectory(prefix='companion-firmware-test-') as temp:
        work=Path(temp);source=work/'test.c'
        source.write_text('int _fltused=0;\n'+source_text,encoding='utf-8')
        library=work/('test.dll' if sys.platform=='win32' else 'test.so')
        flags=['-O2','-Wall','-Wextra','-Werror','-I'+str(root/'firmware/main')]
        if sys.platform=='win32':
            obj=work/'test.obj'
            subprocess.run([args.cc,'--target=x86_64-pc-windows-msvc','-ffreestanding',
                            '-fno-stack-protector',*flags,'-c',str(source),'-o',str(obj)],check=True)
            subprocess.run([args.linker,'-flavor','link','/dll','/noentry','/nodefaultlib',
                            *['/export:'+name for name in exports],'/out:'+str(library),str(obj)],check=True)
        else:
            subprocess.run([args.cc,'-dynamiclib' if sys.platform=='darwin' else '-shared',
                            '-fPIC',*flags,str(source),'-o',str(library)],check=True)
        module=ctypes.CDLL(str(library))
        try:yield module
        finally:
            if sys.platform=='win32':
                kernel=ctypes.WinDLL('kernel32',use_last_error=True)
                kernel.FreeLibrary.argtypes=[ctypes.c_void_p]
                assert kernel.FreeLibrary(module._handle)
