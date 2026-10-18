"""Package only curated public downloads; no QA logs, NVS or SD images."""
import hashlib,json,shutil,stat,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
VERSION="1.0.9"
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def zip_tree(target,folder):
 with zipfile.ZipFile(target,"w",zipfile.ZIP_DEFLATED,compresslevel=9) as archive:
  for source in sorted(folder.rglob("*")):
   if source.is_file():
    name=source.relative_to(folder).as_posix()
    entry=zipfile.ZipInfo(name);entry.date_time=(2026,10,18,0,0,0);entry.create_system=3
    entry.external_attr=((0o755 if name.endswith("/Monitor") or name.endswith("/xz") else 0o644)|stat.S_IFREG)<<16
    entry.compress_type=zipfile.ZIP_DEFLATED;archive.writestr(entry,source.read_bytes())
def main():
 release=ROOT/"dist/public-release";release.mkdir(parents=True,exist_ok=True)
 installer=ROOT/"dist/System Companion Device Installer.exe"
 shutil.copy2(installer,release/"SystemCompanion-Installer.exe")
 device=ROOT/"dist/DEVICE";manifest=json.loads((device/"manifest.json").read_text())
 assert manifest["version"]==VERSION
 for name,digest in manifest["files"].items():assert sha(device/name)==digest,name
 zip_tree(release/"SystemCompanion-Clients.zip",device)
 firmware=ROOT/".local/public-firmware";firmware.mkdir(exist_ok=True)
 for source,name in [("bootloader/bootloader.bin","bootloader.bin"),("partition_table/partition-table.bin","partition-table.bin"),("system_companion.bin","system_companion.bin"),("storage.bin","storage.bin")]:
  shutil.copy2(ROOT/"firmware/build"/source,firmware/name)
 image=json.loads((ROOT/".local/installer-build/payload/image.json").read_text())
 (firmware/"image.json").write_text(json.dumps(image,indent=2),encoding="utf-8")
 (firmware/"README.md").write_text("""# System Companion firmware images

For Waveshare ESP32-S3-Touch-AMOLED-1.43-B only: ESP32-S3, 16 MB flash, 8 MB PSRAM.

The recommended installation method is the single SystemCompanion-Installer.exe
from the matching release. This archive is intended for developers using esptool.

Offsets (also in image.json):
- 0x000000 bootloader.bin
- 0x008000 partition-table.bin
- 0x010000 system_companion.bin
- 0x300000 storage.bin

Do not write the application at offset zero or erase the full chip to update.
These regions preserve the NVS settings partition. The external MoniStore SD
card is separate and is not part of these images. Close Monitor and eject
MoniStore before flashing. Keep USB connected until verification finishes.

Hardware/build/verification documentation and original source:
https://github.com/himwithnoname-boop/System-Companion/tree/v1.0.9
Matching dependency sources: SystemCompanion-ThirdPartySource.zip in the release.
Original code is MIT; dependencies retain their own terms. Preserve the
included THIRD_PARTY_NOTICES.md and licenses folder when redistributing.
""",encoding="utf-8")
 shutil.copy2(ROOT/"THIRD_PARTY_NOTICES.md",firmware/"THIRD_PARTY_NOTICES.md")
 shutil.copytree(ROOT/"licenses",firmware/"licenses",dirs_exist_ok=True)
 shutil.copy2(ROOT/"LICENSE",firmware/"LICENSE")
 zip_tree(release/"SystemCompanion-Firmware.zip",firmware)
 zip_tree(release/"SystemCompanion-ThirdPartySource.zip",ROOT/".local/public-sources")
 for name in ["LICENSE","THIRD_PARTY_NOTICES.md"]:shutil.copy2(ROOT/name,release/name)
 names=["SystemCompanion-Installer.exe","SystemCompanion-Clients.zip","SystemCompanion-Firmware.zip","SystemCompanion-ThirdPartySource.zip","LICENSE","THIRD_PARTY_NOTICES.md"]
 checksums="".join(sha(release/name)+"  "+name+"\n" for name in names)
 (release/"SHA256SUMS.txt").write_text(checksums,encoding="ascii")
 receipt=dict(version=VERSION,firmware=image["firmware_version"],files=[dict(name=name,bytes=(release/name).stat().st_size,sha256=sha(release/name)) for name in names],clientPlatforms=manifest["platforms"])
 (release/"release.json").write_text(json.dumps(receipt,indent=2),encoding="utf-8")
 print(json.dumps(receipt))
if __name__=="__main__":main()
