"""Build an offline, all-platform package that fits the 13 MiB USB partition."""
import argparse
import ctypes
import hashlib
import io
import json
import lzma
from pathlib import Path
import shutil
import stat
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]

LINUX = '''#!/bin/sh
set -eu
umask 077
here=$(CDPATH= cd "$(dirname "$0")" && pwd -P)
case $(uname -m) in
  x86_64|amd64) arch=amd64;;
  aarch64|arm64) arch=arm64;;
  *) printf 'System Companion: unsupported Linux architecture.\\n' >&2; exit 1;;
esac
base=${XDG_CACHE_HOME:-"$HOME/.cache"}/system-companion
mkdir -p "$base"
work=$(mktemp -d "$base/run.XXXXXXXX")
cleanup() {
  rm -f "$work/xz" "$work/clients.tar" "$work/linux/$arch/Monitor"
  rmdir "$work/linux/$arch" "$work/linux" "$work" 2>/dev/null || :
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# Copying the small decoder also handles USB volumes mounted with noexec.
cp "$here/tools/$arch/xz" "$work/xz"
chmod 700 "$work/xz"
"$work/xz" "$here/../clients.tar.xz" > "$work/clients.tar"
tar -xf "$work/clients.tar" -C "$work" "linux/$arch/Monitor"
rm -f "$work/clients.tar" "$work/xz"
chmod 700 "$work/linux/$arch/Monitor"
"$work/linux/$arch/Monitor" "$@"
'''

MAC = '''#!/bin/sh
set -eu
umask 077
here=$(CDPATH= cd "$(dirname "$0")" && pwd -P)
case $(uname -m) in
  arm64) arch=arm64;;
  *) printf 'System Companion: this macOS client requires Apple Silicon.\\n' >&2; exit 1;;
esac
work=$(mktemp -d "${TMPDIR:-/tmp}/system-companion.XXXXXXXX")
cleanup() {
  rm -f "$work/darwin/$arch/Monitor"
  rmdir "$work/darwin/$arch" "$work/darwin" "$work" 2>/dev/null || :
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# macOS's built-in bsdtar reads XZ; no external app is downloaded.
/usr/bin/tar -xf "$here/../../../../clients.tar.xz" -C "$work" "darwin/$arch/Monitor"
chmod 700 "$work/darwin/$arch/Monitor"
"$work/darwin/$arch/Monitor" "$@"
'''

README = '''SYSTEM COMPANION {version}

Everything required to launch the client is carried on this USB drive.
No internet download, client installation, or microSD card is required.
Launchers unpack the selected client into a private temporary folder on the
computer, then remove that folder when the client exits. No drivers are
installed by the launchers. Optional Windows startup remains an explicit
tray setting and creates a stable per-user copy only when enabled.

WINDOWS x64 / ARM64: double-click Windows\\Monitor.exe. Right-click the ring tray
icon for Dashboard, Reconnect, Run on startup, or Exit.

LINUX x64 / ARM64: run Linux/Monitor. If your USB volume does not allow
executing files, open a terminal in Linux and run: sh ./Monitor
The matching architecture and decompressor are bundled. No xz package is
needed. On desktops supporting StatusNotifier/AppIndicator, the ring tray
provides Dashboard, Reconnect, optional Run on startup when connected, and
Exit. The dashboard also provides Reconnect display and Exit monitor.
Serial access may require your distribution's serial group or permission.
For Ubuntu: sudo usermod -aG dialout "$USER", then sign out and back in.

MAC Apple Silicon: open macOS/Monitor.app. If macOS blocks this
unsigned app, approve it using your normal security settings. The built-in
macOS archive reader unpacks the native ARM64 client; no downloaded tool is
required. The menu bar icon provides Dashboard, Reconnect, checked opt-in\nRun on startup when connected, and Exit. The dashboard has the same controls.

Tap the centre for temperature, CPU load, GPU load, RAM usage,
download/upload, and stacked CPU/GPU temperatures. Settings persist on the
device. Auto rotation uses the built-in accelerometer.
The dashboard lets you show or hide individual GPUs. Choices for the four
most recently customised computers are saved on the device; new GPUs start
visible. Text adapts to the selected GPUs. Each GPU summary shows its name,
temperature, overall load, memory and power. The GPU activity bar averages
enabled GPUs reporting load; disabled GPUs and missing readings are excluded.
GPU0 is reserved for detected integrated/onboard graphics. Discrete GPUs
start at GPU1. GPU0 is absent when no integrated GPU is identified.

Unavailable readings show --. Each detected GPU has its own readings.
Windows uses native graphics-driver interfaces; Linux uses hwmon, thermal
zones and DRM; macOS reads Apple SMC and IOAccelerator when exposed.
CPU die temperature is not available without privileged driver access on
all Windows systems. Existing compatible sensor providers are used first;
the optional packaged CPU helper uses existing PawnIO only when explicitly
enabled. No sensor driver is installed automatically. Generic thermal
zones are labelled separately and are not presented as CPU die readings. A VM measures its guest and usually has no physical temperature
or GPU sensors. Startup on macOS uses a per-user login item and needs no admin.

This volume is read-only. Update it through the firmware storage updater.
An inserted microSD appears as an additional writable MoniStore drive
(MONISTORE on FAT systems). It never replaces this client drive. The card
is never formatted automatically. Safely eject it before removal. Themes,
brightness and display preferences stay in device flash; each dashboard
loads them on connection. Dependency notices are
in Licenses. Executable and archive hashes are listed in manifest.json.
'''

def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def standard_user_sfx(source, destination):
    # The upstream 32-bit SFX has no manifest, so Windows installer detection
    # otherwise requests elevation. Embed asInvoker before appending data.
    shutil.copyfile(source, destination)
    manifest = b'''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
<trustInfo xmlns="urn:schemas-microsoft-com:asm.v3"><security><requestedPrivileges>
<requestedExecutionLevel level="asInvoker" uiAccess="false"/>
</requestedPrivileges></security></trustInfo></assembly>'''
    kernel = ctypes.WinDLL('kernel32', use_last_error=True)
    kernel.BeginUpdateResourceW.argtypes = [ctypes.c_wchar_p, ctypes.c_int]
    kernel.BeginUpdateResourceW.restype = ctypes.c_void_p
    kernel.UpdateResourceW.argtypes = [ctypes.c_void_p, ctypes.c_void_p, ctypes.c_void_p, ctypes.c_ushort, ctypes.c_void_p, ctypes.c_uint]
    kernel.EndUpdateResourceW.argtypes = [ctypes.c_void_p, ctypes.c_int]
    handle = kernel.BeginUpdateResourceW(str(destination), False)
    if not handle: raise ctypes.WinError(ctypes.get_last_error())
    buffer = ctypes.create_string_buffer(manifest)
    if not kernel.UpdateResourceW(handle, 24, 1, 1033, buffer, len(manifest)):
        error = ctypes.get_last_error(); kernel.EndUpdateResourceW(handle, True)
        raise ctypes.WinError(error)
    if not kernel.EndUpdateResourceW(handle, False): raise ctypes.WinError(ctypes.get_last_error())
    return destination.read_bytes()

def archive(destination, members):
    data = io.BytesIO()
    with tarfile.open(fileobj=data, mode='w', format=tarfile.USTAR_FORMAT) as tar:
        for name, source in members:
            content = source.read_bytes()
            entry = tarfile.TarInfo(name)
            entry.size, entry.mode, entry.mtime = len(content), 0o700, 0
            tar.addfile(entry, io.BytesIO(content))
    packed = lzma.compress(data.getvalue(), filters=[
        {'id': lzma.FILTER_X86}, {'id': lzma.FILTER_LZMA2, 'preset': 9}])
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_bytes(packed)
    # Check every architecture before committing the package.
    with tarfile.open(fileobj=io.BytesIO(lzma.decompress(packed))) as tar:
        assert tar.getnames() == [name for name, _ in members]
        for name, source in members:
            assert tar.extractfile(name).read() == source.read_bytes()

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--sdk', type=Path, default=ROOT / '.local/lzma-sdk')
    args = parser.parse_args()
    source = ROOT / 'dist/SYSTEM_MONITOR'
    sevenzip, sfx = args.sdk / 'bin/x64/7zr.exe', args.sdk / 'bin/7zS2.sfx'
    for path in [sevenzip, sfx, ROOT / '.local/portable-tools/xz-amd64', ROOT / '.local/portable-tools/xz-arm64', ROOT / '.local/portable-tools/windows-launcher.exe']:
        if not path.is_file():
            raise SystemExit(f'Missing build tool: {path}; run tools/build-portable.ps1 first.')
    version = source.joinpath('macOS/Monitor.app/Contents/Info.plist').read_text().split('<key>CFBundleVersion</key><string>')[1].split('</string>')[0]
    with tempfile.TemporaryDirectory(prefix='device-package-', dir=ROOT / '.local') as folder:
        stage = Path(folder) / 'DEVICE'
        stage.mkdir()
        windows = Path(folder) / 'windows.7z'
        # Temporary setup executables and diagnostic logs are never distributed.
        windows_source = Path(folder) / 'windows-source'
        windows_source.mkdir()
        shutil.copyfile(ROOT / '.local/portable-tools/windows-launcher.exe', windows_source / 'Monitor.exe')
        for arch, folder_name in [('amd64', 'Windows'), ('arm64', 'Windows-arm64')]:
            native = windows_source / arch
            native.mkdir()
            shutil.copyfile(source / folder_name / 'Monitor.exe', native / 'Monitor.exe')
        sensor_source = source / 'Windows/Sensors'
        if sensor_source.is_dir():
            sensors = windows_source / 'Sensors'
            sensors.mkdir()
            for library in sensor_source.iterdir():
                if library.is_file() and (library.name == 'Sensors.exe' or library.suffix.lower() == '.dll'):
                    shutil.copyfile(library, sensors / library.name)
        subprocess.run([str(sevenzip), 'a', str(windows), str(windows_source / '*'), '-t7z', '-mx=9', '-ms=on', '-mmt=2', '-y', '-bd'], check=True, stdout=subprocess.DEVNULL)
        (stage / 'Windows').mkdir()
        stub = standard_user_sfx(sfx, Path(folder) / 'launcher.exe')
        (stage / 'Windows/Monitor.exe').write_bytes(stub + windows.read_bytes())
        archive(stage / 'clients.tar.xz', [
            ('linux/amd64/Monitor', source / 'Linux/Monitor'),
            ('linux/arm64/Monitor', source / 'Linux-arm64/Monitor'),
            ('darwin/arm64/Monitor', source / 'macOS/Monitor.app/Contents/MacOS/Monitor')])
        (stage / 'Linux').mkdir()
        (stage / 'Linux/Monitor').write_text(LINUX, encoding='utf-8', newline='\n')
        for arch in ['amd64', 'arm64']:
            decoder = stage / f'Linux/tools/{arch}/xz'
            decoder.parent.mkdir(parents=True)
            shutil.copyfile(ROOT / f'.local/portable-tools/xz-{arch}', decoder)
        contents = stage / 'macOS/Monitor.app/Contents'
        (contents / 'MacOS').mkdir(parents=True)
        (contents / 'MacOS/Monitor').write_text(MAC, encoding='utf-8', newline='\n')
        shutil.copyfile(source / 'macOS/Monitor.app/Contents/Info.plist', contents / 'Info.plist')
        (stage / 'README.txt').write_text(README.format(version=version), encoding='utf-8', newline='\n')
        shutil.copytree(source / 'Licenses', stage / 'Licenses')
        shutil.copyfile(ROOT / 'tools/vendor/lzma/NOTICE', stage / 'Licenses/LZMA-SDK.txt')
        manifest = {'version': version, 'platforms': ['windows-amd64', 'windows-arm64', 'linux-amd64', 'linux-arm64', 'darwin-arm64'],
                    'files': {p.relative_to(stage).as_posix(): digest(p) for p in sorted(stage.rglob('*')) if p.is_file()},
                    'clients': {p: digest(source / p) for p in ['Windows/Monitor.exe', 'Windows-arm64/Monitor.exe', 'Linux/Monitor', 'Linux-arm64/Monitor',
                        'macOS/Monitor.app/Contents/MacOS/Monitor']}}
        (stage / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')
        occupied = sum(((p.stat().st_size + 4095) // 4096) * 4096 for p in stage.rglob('*') if p.is_file())
        if occupied + 256 * 1024 > 0xd00000:
            raise SystemExit(f'Portable package too large for internal flash: {occupied} bytes plus filesystem reserve')
        # Replace only the generated device package, after all checks pass.
        destination = ROOT / 'dist/DEVICE'
        assert destination.resolve() == ROOT / 'dist/DEVICE'
        if destination.exists():
            # Go's module-cache license files can carry the Windows read-only
            # attribute. This directory contains only generated package files.
            for existing in destination.rglob('*'):
                if existing.is_file():
                    existing.chmod(stat.S_IREAD | stat.S_IWRITE)
            shutil.rmtree(destination)
        shutil.copytree(stage, destination)
        print(json.dumps({'allFiveClientsOnDevice': True, 'allocatedFileBytes': occupied, 'partitionBytes': 0xd00000,
            'sfxBytes': (stage / 'Windows/Monitor.exe').stat().st_size,
            'sharedUnixArchiveBytes': (stage / 'clients.tar.xz').stat().st_size}, indent=2))

if __name__ == '__main__':
    main()
