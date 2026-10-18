using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.IO.Compression;
using System.Reflection;
using System.Security.Cryptography;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
using System.Web.Script.Serialization;

[assembly: AssemblyTitle("System Companion Device Installer")]
[assembly: AssemblyDescription("Offline ESP32-S3 firmware and client installer")]
[assembly: AssemblyCompany("System Companion")]
[assembly: AssemblyProduct("System Companion Device Installer")]
[assembly: AssemblyVersion("1.0.9.0")]
[assembly: AssemblyFileVersion("1.0.9.0")]

class DeviceChoice {
 public string Port, Serial, Description;
 public override string ToString() { return Port+"  ·  "+Description; }
}
class Installer : Form {
 const string PayloadHash = "__PAYLOAD_HASH__";
 string work, autoPort, report, preview;
 bool busy;
 ComboBox ports = new ComboBox();
 Button refresh = new Button(), flash = new Button();
 Label state = new Label();
 ProgressBar progress = new ProgressBar();
 TextBox details = new TextBox();
 List<string> events = new List<string>();
 System.Windows.Forms.Timer watcher = new System.Windows.Forms.Timer();
 string portSnapshot = "";
 static JavaScriptSerializer json = new JavaScriptSerializer();
 public Installer(string[] args) {
  for(int i=0;i<args.Length;i++) {
   if(args[i]=="--flash" && i+1<args.Length) autoPort=args[++i];
   else if(args[i]=="--report" && i+1<args.Length) report=args[++i];
   else if(args[i]=="--preview" && i+1<args.Length) preview=args[++i];
  }
  Text="System Companion · Device Installer";
  ClientSize=new Size(680,520); MinimumSize=new Size(680,520);
  StartPosition=FormStartPosition.CenterScreen;
  Font=new Font("Segoe UI",10); BackColor=Color.FromArgb(247,249,252);
  Icon=System.Drawing.Icon.ExtractAssociatedIcon(Application.ExecutablePath);
  Label title=new Label {Text="Set up your System Companion",Font=new Font("Segoe UI Semibold",20),AutoSize=true,Location=new Point(26,22)};
  Label subtitle=new Label {Text="Firmware 1.0.11  ·  Clients 1.0.9  ·  Offline installation",AutoSize=true,Location=new Point(28,69),ForeColor=Color.DimGray};
  Label target=new Label {Text="For Waveshare ESP32-S3-Touch-AMOLED-1.43-B with 16 MB flash.\nConnect one display by USB. Eject MoniStore and exit Monitor before installing.",Location=new Point(28,106),Size=new Size(620,48)};
  ports.Location=new Point(28,169); ports.Size=new Size(482,30); ports.DropDownStyle=ComboBoxStyle.DropDownList;ports.Anchor=AnchorStyles.Top|AnchorStyles.Left|AnchorStyles.Right;
  refresh.Text="Refresh";refresh.Location=new Point(524,167);refresh.Size=new Size(126,32);refresh.Anchor=AnchorStyles.Top|AnchorStyles.Right;
  state.Location=new Point(28,220); state.Size=new Size(622,46);state.Text="Preparing the offline installer…";state.Anchor=AnchorStyles.Top|AnchorStyles.Left|AnchorStyles.Right;
  progress.Location=new Point(28,274);progress.Size=new Size(622,10);progress.Anchor=AnchorStyles.Top|AnchorStyles.Left|AnchorStyles.Right;
  details.Location=new Point(28,302);details.Size=new Size(622,134);details.Multiline=true;details.ReadOnly=true;details.ScrollBars=ScrollBars.Vertical;details.BackColor=Color.White;details.BorderStyle=BorderStyle.FixedSingle;details.Font=new Font("Consolas",8);details.Anchor=AnchorStyles.Top|AnchorStyles.Bottom|AnchorStyles.Left|AnchorStyles.Right;
  Label preserve=new Label {Text="Includes Windows, Linux and macOS clients. Keeps settings and SD card data.",Location=new Point(28,457),Size=new Size(445,38),ForeColor=Color.DimGray,Font=new Font("Segoe UI",9),Anchor=AnchorStyles.Bottom|AnchorStyles.Left};
  flash.Text="Install / Update";flash.Location=new Point(500,455);flash.Size=new Size(150,38);flash.BackColor=Color.FromArgb(0,96,176);flash.ForeColor=Color.White;flash.FlatStyle=FlatStyle.Flat;flash.Anchor=AnchorStyles.Bottom|AnchorStyles.Right;
  Controls.AddRange(new Control[]{title,subtitle,target,ports,refresh,state,progress,details,preserve,flash});
  refresh.Enabled=false;flash.Enabled=false;
  refresh.Click+=async (s,e)=>await Scan();
  flash.Click+=async (s,e)=>await Install();
  Shown+=async (s,e)=>{
   try { await Task.Run(()=>Prepare());await Scan();watcher.Start();
    if(preview!=null) {
     using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(0,0,Width,Height));bitmap.Save(preview,System.Drawing.Imaging.ImageFormat.Png);}
     FinishReport(0);if(report==null)Close();return;
    }
    if(autoPort!=null) {for(int i=0;i<ports.Items.Count;i++) if(((DeviceChoice)ports.Items[i]).Port==autoPort) ports.SelectedIndex=i;
     if(ports.SelectedItem==null || ((DeviceChoice)ports.SelectedItem).Port!=autoPort) throw new Exception("Requested device is not connected.");
     await Install();
    }
   } catch(Exception error) {Fail(error.Message);FinishReport(1);}
  };
  watcher.Interval=2000;
  watcher.Tick+=async(s,e)=>{if(!busy){string current=String.Join(",",System.IO.Ports.SerialPort.GetPortNames());if(current!=portSnapshot)await Scan();}};
  FormClosing+=(s,e)=>{if(busy){e.Cancel=true;state.Text="Installation is in progress. Keep USB connected until verification finishes.";}};
  FormClosed+=(s,e)=>{watcher.Stop();Cleanup();};
 }
 void Prepare() {
  work=Path.Combine(Path.GetTempPath(),"SystemCompanionInstaller-"+Guid.NewGuid().ToString("N"));
  Directory.CreateDirectory(work);
  string archive=Path.Combine(work,"payload.zip");
  using(Stream stream=Assembly.GetExecutingAssembly().GetManifestResourceStream("payload.zip"))
  using(FileStream file=File.Create(archive)) stream.CopyTo(file);
  using(var sha=SHA256.Create()) using(var file=File.OpenRead(archive)) {
   string actual=BitConverter.ToString(sha.ComputeHash(file)).Replace("-","").ToLowerInvariant();
   if(actual!=PayloadHash) throw new Exception("Installer payload verification failed. Use a fresh copy of the installer.");
  }
  ZipFile.ExtractToDirectory(archive,work);File.Delete(archive);
 }
 async Task<int> Worker(string args, Action<Dictionary<string,object>> consume) {
  var start=new ProcessStartInfo(Path.Combine(work,"python.exe"),"-I -u \""+Path.Combine(work,"flash_worker.py")+"\" "+args);
  start.UseShellExecute=false;start.CreateNoWindow=true;start.RedirectStandardOutput=true;start.RedirectStandardError=true;start.StandardOutputEncoding=Encoding.UTF8;start.StandardErrorEncoding=Encoding.UTF8;start.WorkingDirectory=work;
  start.EnvironmentVariables["PYTHONIOENCODING"]="utf-8";
  bool protocolError=false;
  using(var process=Process.Start(start)) {
   var stderr=process.StandardError.ReadToEndAsync();
   string line;while((line=await process.StandardOutput.ReadLineAsync())!=null) {
    events.Add(line);
    try{consume(json.Deserialize<Dictionary<string,object>>(line));}
    catch(Exception error){protocolError=true;Append("Worker output: "+error.Message);}
   }
   await Task.Run(()=>process.WaitForExit());
   string errors=await stderr;if(!String.IsNullOrWhiteSpace(errors))Append(errors);
   return process.ExitCode==0 && protocolError ? 1 : process.ExitCode;
  }
 }
 void Append(string text) { details.AppendText(text+Environment.NewLine); }
 void Fail(string text) {state.Text="Installation could not complete. See details below.";Append(text);progress.Style=ProgressBarStyle.Blocks;}
 async Task Scan() {
  busy=true;refresh.Enabled=false;flash.Enabled=false;
  try{
   string old=(ports.SelectedItem as DeviceChoice)==null ? null:((DeviceChoice)ports.SelectedItem).Port;
   int code=await Worker("--list",msg=>{
    if((string)msg["event"]=="devices") {
     ports.Items.Clear();
     foreach(object value in (System.Collections.ArrayList)msg["devices"]) {
      var row=(Dictionary<string,object>)value;
      ports.Items.Add(new DeviceChoice{Port=(string)row["port"],Serial=(string)row["serial"],Description=(string)row["description"]});
     }
     if(ports.Items.Count>0) {ports.SelectedIndex=0;for(int i=0;i<ports.Items.Count;i++) if(((DeviceChoice)ports.Items[i]).Port==old)ports.SelectedIndex=i;}
     state.Text=ports.Items.Count==0?"Connect the display by USB. For a blank board, hold BOOT, press RESET, then release BOOT.":"Ready: "+ports.SelectedItem.ToString()+". Choose Install / Update.";
    } else if((string)msg["event"]=="error") throw new Exception((string)msg["text"]);
   });
   if(code!=0)Fail("The bundled flashing tools could not start.");
   portSnapshot=String.Join(",",System.IO.Ports.SerialPort.GetPortNames());
  } finally {busy=false;refresh.Enabled=true;flash.Enabled=ports.Items.Count>0;}
 }
 async Task Install() {
  DeviceChoice device=ports.SelectedItem as DeviceChoice;if(device==null)return;
  watcher.Stop();busy=true;flash.Enabled=false;refresh.Enabled=false;ports.Enabled=false;
  progress.Style=ProgressBarStyle.Marquee;bool completed=false;
  try {
   Append("Installing to "+device.Port+". Preserve settings; do not erase SD.");
   int code=await Worker("--flash "+device.Port+" --serial \""+device.Serial.Replace("\"","")+"\"",msg=>{
    string kind=(string)msg["event"];
    if(kind=="log") {Append((string)msg["text"]);if(msg.ContainsKey("percent")&&msg["percent"]!=null){progress.Style=ProgressBarStyle.Blocks;progress.Value=Math.Max(0,Math.Min(100,Convert.ToInt32(msg["percent"])));}}
    else if(kind=="stage") {state.Text=(string)msg["text"];Append(state.Text);}
    else if(kind=="error") Fail((string)msg["text"]);
    else if(kind=="complete") {completed=true;progress.Style=ProgressBarStyle.Blocks;progress.Value=100;state.Text=(string)msg["text"];Append(state.Text);}
   });
   if(code!=0 || !completed) {Fail("Please check the connection and retry. Existing settings and SD were not erased.");FinishReport(1);}
   else FinishReport(0);
  } catch(Exception error) {Fail(error.Message);FinishReport(1);}
  finally {busy=false;flash.Enabled=true;refresh.Enabled=true;ports.Enabled=true;watcher.Start();}
 }
 void FinishReport(int code) {
  if(report==null)return;
  File.WriteAllText(report,json.Serialize(new{exitCode=code,events=events}),Encoding.UTF8);
  Environment.ExitCode=code;
  BeginInvoke(new Action(()=>{busy=false;Close();}));
 }
 void Cleanup() {try {if(work!=null && Path.GetFileName(work).StartsWith("SystemCompanionInstaller-"))Directory.Delete(work,true);}catch{}}
 [STAThread] static int Main(string[] args) {
  Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
  using(var mutex=new System.Threading.Mutex(true,@"Local\SystemCompanionDeviceInstaller")) {
   if(!mutex.WaitOne(0)) {MessageBox.Show("System Companion Device Installer is already open.","System Companion");return 1;}
   var form=new Installer(args);
   try {
    if(args.Length==2 && args[0]=="--self-test") {
     form.Prepare();var output=new StringBuilder();
     var start=new ProcessStartInfo(Path.Combine(form.work,"python.exe"),"-I -u \""+Path.Combine(form.work,"flash_worker.py")+"\" --verify"){UseShellExecute=false,CreateNoWindow=true,RedirectStandardOutput=true,RedirectStandardError=true,StandardOutputEncoding=Encoding.UTF8};
     using(var p=Process.Start(start)){string data=p.StandardOutput.ReadToEnd();string err=p.StandardError.ReadToEnd();p.WaitForExit();File.WriteAllText(args[1],data+err,Encoding.UTF8);return p.ExitCode;}
    }
    Application.Run(form);return Environment.ExitCode;
   } catch(Exception error) {MessageBox.Show(error.Message,"System Companion Device Installer",MessageBoxButtons.OK,MessageBoxIcon.Error);return 1;}
   finally {form.Cleanup();}
  }
 }
}
