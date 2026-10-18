import importlib.util,json,tempfile,unittest,hashlib
from pathlib import Path
from unittest.mock import patch
ROOT=Path.cwd()
spec=importlib.util.spec_from_file_location("flash_worker",ROOT/"tools/installer/flash_worker.py")
w=importlib.util.module_from_spec(spec);spec.loader.exec_module(w)
class WorkerTests(unittest.TestCase):
 def test_target(self):
  class Esp:
   CHIP_NAME="ESP32-S3"
   secure_download_mode=False
   stub_is_disabled=False
   def get_secure_boot_enabled(self):return False
   def get_flash_encryption_enabled(self):return False
   def flash_spi_attach(self,value):assert value==0
   def flash_id(self):return 0x1840ef
  w.confirm_target(Esp())
  with patch.object(Esp,"secure_download_mode",True):
   with self.assertRaisesRegex(RuntimeError,"protected"):w.confirm_target(Esp())
  with patch.object(Esp,"CHIP_NAME","ESP32"):
   with self.assertRaisesRegex(RuntimeError,"ESP32-S3"):w.confirm_target(Esp())
  with patch.object(Esp,"flash_id",lambda _:0xffffff):
   with self.assertRaisesRegex(RuntimeError,"identify"):w.confirm_target(Esp())
  with patch.object(Esp,"flash_id",lambda _:0x1740ef):
   with self.assertRaisesRegex(RuntimeError,"16 MB"):w.confirm_target(Esp())
 def test_bundle(self):
  with tempfile.TemporaryDirectory() as d:
   root=Path(d);images=[]
   for offset in (0,0x8000,0x10000,0x300000):
    path=root/str(offset);path.write_bytes(b"verified")
    images.append(dict(file=path.name,offset=hex(offset),bytes=8,sha256=hashlib.sha256(b"verified").hexdigest()))
   manifest=dict(chip="esp32s3",flash_bytes=16*1024*1024,images=images)
   def save(): (root/"image.json").write_text(json.dumps(manifest))
   save();self.assertEqual(w.verify_bundle(root),manifest)
   (root/"0").write_bytes(b"modified")
   with self.assertRaisesRegex(ValueError,"Damaged"):w.verify_bundle(root)
   (root/"0").write_bytes(b"verified");manifest["images"][0]["offset"]="0x9000";save()
   with self.assertRaisesRegex(ValueError,"layout"):w.verify_bundle(root)
   manifest["images"][0]["offset"]="0x0";manifest["images"].pop();save()
   with self.assertRaisesRegex(ValueError,"Incomplete"):w.verify_bundle(root)
 def test_wrong_port_and_identity(self):
  class Port:device="COM7";serial_number="44:1b";vid=0x303a;pid=0x4002
  with patch.object(w.list_ports,"comports",return_value=[Port()]):
   self.assertEqual(w.get_port("COM7","441B").device,"COM7")
   with self.assertRaisesRegex(RuntimeError,"different device"):w.get_port("COM7","other")
   with self.assertRaisesRegex(RuntimeError,"disconnected"):w.get_port("COM8","")
 def test_already_in_native_bootloader(self):
  class Port: device="COM9";vid=0x303a;pid=0x0009
  self.assertEqual(w.enter_bootloader(Port()),("COM9","no_reset"))
 def test_boot_identity(self):
  self.assertEqual(w.normal_serial("AA:BB:CC:DD:EE:FF"),w.normal_serial("AABBCCDDEEFF"))
unittest.main()
