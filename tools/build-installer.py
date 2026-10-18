"""Build one offline Windows GUI executable from the existing local toolchains."""
import argparse, hashlib, importlib.metadata, json, os, shutil, subprocess, sys, zipfile
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
def sha(p): return hashlib.sha256(p.read_bytes()).hexdigest()
def main():
    parser=argparse.ArgumentParser()
    parser.add_argument("--flasher-python",type=Path,required=True)
    args=parser.parse_args()
    build=ROOT/".local/installer-build"
    build.mkdir(parents=True,exist_ok=True)
    (ROOT/"dist/Verification").mkdir(parents=True,exist_ok=True)
    stage=build/"payload"
    if stage.exists():
        assert stage.resolve()==ROOT/".local/installer-build/payload"
        shutil.rmtree(stage)
    stage.mkdir()
    query="import sys,json,importlib.metadata as m; print(json.dumps(dict(base=sys.base_prefix,site=str(m.distribution('esptool').locate_file('')),version=list(sys.version_info[:2]))))"
    info=json.loads(subprocess.check_output([str(args.flasher_python),"-c",query],text=True))
    base=Path(info["base"]); site=Path(info["site"])
    if info["version"]!=[3,12]: raise RuntimeError("Expected the tested Python 3.12 flashing runtime")
    for name in ("python.exe","python312.dll","python3.dll","vcruntime140.dll","vcruntime140_1.dll"):
        shutil.copy2(base/name,stage/name)
    shutil.copytree(base/"DLLs",stage/"DLLs",ignore=shutil.ignore_patterns("__pycache__","*.pdb","*.lib"))
    # No development tooling, pip, host site-packages, Tk, or console is installed.
    with zipfile.ZipFile(stage/"python312.zip","w",zipfile.ZIP_DEFLATED,compresslevel=9) as archive:
        excluded={"site-packages","test","tests","idlelib","tkinter","turtledemo","ensurepip","venv","__pycache__"}
        for path in sorted((base/"Lib").rglob("*")):
            rel=path.relative_to(base/"Lib")
            if path.is_file() and not (set(rel.parts)&excluded) and path.suffix in {".py",".pickle",".txt"}:
                archive.write(path,rel.as_posix())
    (stage/"python312._pth").write_text("python312.zip\nDLLs\nLib/site-packages\n.\n",encoding="ascii")
    packages=stage/"Lib/site-packages";packages.mkdir(parents=True)
    dependencies=["esptool","bitstring","bitarray","cryptography","cffi","pycparser","ecdsa","six","pyserial","reedsolo","PyYAML","intelhex"]
    inventory=[]
    # Query each installed distribution through its own environment.
    code="import json,importlib.metadata as m; print(json.dumps([{ 'name':n,'version':m.version(n),'files':[str(p) for p in m.files(n)]} for n in "+repr(dependencies)+"]))"
    distributions=json.loads(subprocess.check_output([str(args.flasher_python),"-c",code],text=True))
    for distribution in distributions:
        inventory.append(dict(name=distribution["name"],version=distribution["version"]))
        for name in distribution["files"]:
            rel=Path(name)
            if rel.is_absolute() or ".." in rel.parts or "__pycache__" in rel.parts or rel.suffix in {".pyc",".pyo"}:continue
            source=site/rel
            if source.is_file():
                dest=packages/rel;dest.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(source,dest)
    shutil.copy2(ROOT/"tools/installer/flash_worker.py",stage/"flash_worker.py")
    fw=ROOT/"firmware/build"
    paths=[("0x0",fw/"bootloader/bootloader.bin","bootloader.bin"),
        ("0x8000",fw/"partition_table/partition-table.bin","partition-table.bin"),
        ("0x10000",fw/"system_companion.bin","system_companion.bin"),
        ("0x300000",fw/"storage.bin","storage.bin")]
    manifest={"product":"System Companion","board":"Waveshare ESP32-S3-Touch-AMOLED-1.43-B",
        "chip":"esp32s3","flash_bytes":16*1024*1024,"firmware_version":"1.0.11","client_version":"1.0.9",
        "preserves":["NVS preferences","MoniStore SD card"],"images":[],"dependencies":inventory}
    for offset,source,name in paths:
        dest=stage/name;shutil.copy2(source,dest)
        manifest["images"].append(dict(offset=offset,file=name,bytes=dest.stat().st_size,sha256=sha(dest)))
    (stage/"image.json").write_text(json.dumps(manifest,indent=2),encoding="utf-8")
    license_dir=stage/"Licenses";license_dir.mkdir()
    shutil.copy2(base/"LICENSE.txt",license_dir/"Python-LICENSE.txt")
    (license_dir/"NOTICE.txt").write_text(
        "System Companion Device Installer bundles Python and unmodified esptool 4.12.0 with dependencies.\n"
        "Package source (.py files), license texts and distribution metadata are in Lib/site-packages.\n"
        "esptool is GPL-2.0-or-later; upstream corresponding source: https://github.com/espressif/esptool/tree/v4.12.0\n"
        "Python source: https://www.python.org/downloads/source/\n"
        "No tool is downloaded or driver installed at runtime.\n",encoding="utf-8")
    probe=subprocess.run([str(stage/"python.exe"),"-I","-u",str(stage/"flash_worker.py"),"--verify"],capture_output=True,text=True)
    (build/"runtime-check.json").write_text(probe.stdout+probe.stderr,encoding="utf-8")
    if probe.returncode: raise RuntimeError("Offline runtime check failed: "+probe.stderr)
    payload=build/"payload.zip"
    with zipfile.ZipFile(payload,"w",zipfile.ZIP_DEFLATED,compresslevel=9) as archive:
        for path in sorted(stage.rglob("*")):
            if path.is_file() and "__pycache__" not in path.parts:archive.write(path,path.relative_to(stage).as_posix())
    source=(ROOT/"tools/installer/Installer.cs").read_text(encoding="utf-8-sig").replace("__PAYLOAD_HASH__",sha(payload))
    generated=build/"Installer.cs";generated.write_text(source,encoding="utf-8")
    output=ROOT/"dist/System Companion Device Installer.exe"
    compiler=Path(os.environ["WINDIR"])/"Microsoft.NET/Framework64/v4.0.30319/csc.exe"
    command=[str(compiler),"/nologo","/target:winexe","/platform:x64","/optimize+",
        "/reference:System.Windows.Forms.dll","/reference:System.Drawing.dll","/reference:System.Web.Extensions.dll",
        "/reference:System.IO.Compression.dll","/reference:System.IO.Compression.FileSystem.dll",
        "/win32manifest:"+str(ROOT/"tools/installer/installer.manifest"),
        "/win32icon:"+str(ROOT/"tools/installer/assets/installer.ico"),
        "/resource:"+str(payload)+",payload.zip","/out:"+str(output),str(generated)]
    subprocess.run(command,check=True)
    receipt={"installer":str(output),"bytes":output.stat().st_size,"sha256":sha(output),"payload_sha256":sha(payload),"manifest":manifest}
    (ROOT/"dist/Verification/1.0.9-installer-build.json").write_text(json.dumps(receipt,indent=2),encoding="utf-8")
    print(json.dumps({k:v for k,v in receipt.items() if k!="manifest"}))
if __name__=="__main__":main()
