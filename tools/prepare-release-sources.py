"""Collect unmodified corresponding upstream source archives for a release."""
import concurrent.futures,hashlib,json,urllib.request,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
def read(url):
 return urllib.request.urlopen(urllib.request.Request(url,headers={"User-Agent":"SystemCompanion-release"}),timeout=120).read()
def main():
 folder=ROOT/".local/public-sources";folder.mkdir(parents=True,exist_ok=True)
 runtime=json.loads((ROOT/".local/installer-build/payload/image.json").read_text())
 rows=[
  ("LibreHardwareMonitor-0.9.6.zip","https://codeload.github.com/LibreHardwareMonitor/LibreHardwareMonitor/zip/3d331e3370efb858411f19511373eff65a218701","MPL-2.0"),
  ("DiskInfoToolkit-1.1.2.zip","https://codeload.github.com/Blacktempel/DiskInfoToolkit/zip/25319eae5781e75bcf141e844ceab2afe94d40ea","MPL-2.0"),
  ("RAMSPDToolkit-1.4.2.zip","https://codeload.github.com/Blacktempel/RAMSPDToolkit/zip/3b47b960e0830fef344624ad5e389675d5f0a1ce","MPL-2.0"),
  ("esptool-legacy-flasher-stub-1.11.1.zip","https://codeload.github.com/espressif/esptool-legacy-flasher-stub/zip/refs/tags/v1.11.1","GPL-2.0-or-later"),
  ("esp-flasher-stub-0.7.0.zip","https://codeload.github.com/espressif/esp-flasher-stub/zip/refs/tags/v0.7.0","MIT OR Apache-2.0"),
 ]
 for dependency in runtime["dependencies"]:
  meta=json.loads(read("https://pypi.org/pypi/"+dependency["name"]+"/"+dependency["version"]+"/json"))
  source=next(row for row in meta["urls"] if row["packagetype"]=="sdist")
  rows.append((source["filename"],source["url"],meta["info"].get("license_expression") or meta["info"].get("license") or "See source licence"))
 def get(row):
  name,url,terms=row;path=folder/name
  if not path.exists():path.write_bytes(read(url))
  return dict(file=name,url=url,sha256=hashlib.sha256(path.read_bytes()).hexdigest(),bytes=path.stat().st_size,licence=terms[:150])
 with concurrent.futures.ThreadPoolExecutor(6) as pool:records=list(pool.map(get,rows))
 index={"description":"Unmodified corresponding sources for redistributed flashing and optional CPU sensor components.","archives":records}
 (ROOT/"third-party/sources.json").write_text(json.dumps(index,indent=2)+"\n",encoding="utf-8")
 (folder/"README.txt").write_text("Unmodified source archives for System Companion release dependencies.\nSee sources.json for pinned origins, SHA-256 checksums and licence identifiers.\nOriginal project source and build scripts are available in the matching GitHub tag.\n",encoding="utf-8")
 (folder/"sources.json").write_text(json.dumps(index,indent=2),encoding="utf-8")
 print(json.dumps({"sourceArchives":len(records),"totalBytes":sum(row["bytes"] for row in records)}))
if __name__=="__main__":main()
