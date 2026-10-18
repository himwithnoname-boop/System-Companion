"""Explicit developer-only download of pinned packaging inputs. No driver installs."""
import argparse,hashlib,json,urllib.request,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
def download(row,path):
 path.parent.mkdir(parents=True,exist_ok=True)
 if not path.exists() or hashlib.sha256(path.read_bytes()).hexdigest()!=row["sha256"]:
  urllib.request.urlretrieve(row["url"],path)
 if hashlib.sha256(path.read_bytes()).hexdigest()!=row["sha256"]:
  raise RuntimeError("Build dependency checksum mismatch: "+path.name)
def main():
 parser=argparse.ArgumentParser()
 parser.add_argument("--sensors",action="store_true",help="download and unpack the optional CPU helper libraries")
 args=parser.parse_args()
 data=json.loads((ROOT/"tools/dependencies.json").read_text())
 sdk=ROOT/".local/downloads/lzma2603.7z";download(data["lzmaSdk"],sdk)
 print("Extract "+str(sdk)+" into .local/lzma-sdk using your archive tool.")
 if args.sensors:
  path=ROOT/".local/downloads/LibreHardwareMonitor.zip";download(data["libreHardwareMonitor"],path)
  target=ROOT/".local/lhm";target.mkdir(parents=True,exist_ok=True)
  with zipfile.ZipFile(path) as archive:
   for entry in archive.infolist():
    dest=(target/entry.filename).resolve()
    if not dest.is_relative_to(target.resolve()):raise RuntimeError("Unsafe archive path")
   archive.extractall(target)
  print("Optional sensor build inputs ready; no sensor driver was installed.")
if __name__=="__main__":main()
