// CPU sensor bridge for an existing PawnIO installation; does not install drivers.
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.Text;
using System.Threading;
using System.IO.Pipes;
using System.Security.AccessControl;
using System.Security.Principal;
using System.Reflection;
using System.Management;
using LibreHardwareMonitor.Hardware;
[assembly: AssemblyFileVersion("1.0.9.0")]
[assembly: AssemblyTitle("Monitor CPU Setup")]
class Sensors {
    const string TaskPrefix="SystemCompanion-CPU-";
    static readonly string[] Libraries={"LibreHardwareMonitorLib.dll","HidSharp.dll","DiskInfoToolkit.dll","RAMSPDToolkit-NDD.dll","Microsoft.Bcl.AsyncInterfaces.dll","Microsoft.Bcl.HashCode.dll","System.Buffers.dll","System.Memory.dll","System.Numerics.Vectors.dll","System.Runtime.CompilerServices.Unsafe.dll","System.Threading.Tasks.Extensions.dll","System.IO.Pipelines.dll"};
    static void ProtectDirectory(string path) {
        if(Directory.Exists(path)&&(File.GetAttributes(path)&FileAttributes.ReparsePoint)!=0) throw new IOException("Sensor directory is a reparse point");
        Directory.CreateDirectory(path);
        var acl=new DirectorySecurity();acl.SetAccessRuleProtection(true,false);
        var admins=new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid,null);
        acl.SetOwner(admins);
        foreach(var sid in new[]{admins,new SecurityIdentifier(WellKnownSidType.LocalSystemSid,null)})
            acl.AddAccessRule(new FileSystemAccessRule(sid,FileSystemRights.FullControl,InheritanceFlags.ContainerInherit|InheritanceFlags.ObjectInherit,PropagationFlags.None,AccessControlType.Allow));
        acl.AddAccessRule(new FileSystemAccessRule(new SecurityIdentifier(WellKnownSidType.AuthenticatedUserSid,null),FileSystemRights.ReadAndExecute,InheritanceFlags.ContainerInherit|InheritanceFlags.ObjectInherit,PropagationFlags.None,AccessControlType.Allow));
        Directory.SetAccessControl(path,acl);
    }
    static string SourceDirectory() {
        string location=Assembly.GetExecutingAssembly().Location;
        string folder=Path.GetDirectoryName(location);
        if(Path.GetFileNameWithoutExtension(location)=="Monitor CPU Setup")folder=Directory.GetParent(Directory.GetParent(folder).FullName).FullName;
        return folder;
    }
    static void SetupLog(string message) {
        try { File.WriteAllText(Path.Combine(SourceDirectory(),"cpu-setup.log"),message,new UTF8Encoding(false)); } catch {}
    }
    static void StopAbandonedReaders(string source,string sid) {
        // Only our exact helper path with a dead Monitor parent is eligible.
        var expected=new List<string>{Path.Combine(source,"Sensors.exe")};
        using(var key=Microsoft.Win32.Registry.LocalMachine.OpenSubKey(@"SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\"+sid)) {
            if(key!=null){string profile=key.GetValue("ProfileImagePath") as string;if(profile!=null)expected.Add(Path.Combine(Environment.ExpandEnvironmentVariables(profile),"AppData","Local","SystemCompanion","Sensors","Sensors.exe"));}
        }
        using(var search=new ManagementObjectSearcher("SELECT ProcessId,ParentProcessId,ExecutablePath FROM Win32_Process WHERE Name='Sensors.exe'")) {
            foreach(ManagementObject row in search.Get()) {
                string path=row["ExecutablePath"] as string;
                if(!expected.Exists(item=>String.Equals(path,item,StringComparison.OrdinalIgnoreCase)))continue;
                int pid=Convert.ToInt32(row["ProcessId"]),parent=Convert.ToInt32(row["ParentProcessId"]);
                using(var process=Process.GetProcessById(pid)) {
                    bool abandoned=false;
                    try {using(var owner=Process.GetProcessById(parent))abandoned=owner.HasExited||owner.StartTime>process.StartTime;}catch(ArgumentException){abandoned=true;}
                    if(abandoned){process.Kill();process.WaitForExit(1500);}
                }
            }
        }
    }
    static int InstallPersistent(string sid) {
        SetupLog("Starting CPU setup");
        using(var identity=WindowsIdentity.GetCurrent()) {
            if(!new WindowsPrincipal(identity).IsInRole(WindowsBuiltInRole.Administrator))return 3;
        }
        SetupLog("Administrator approval accepted; preparing protected helper");
        new SecurityIdentifier(sid); // validate before constructing paths or XML
        string source=SourceDirectory();
        StopAbandonedReaders(source,sid);
        string root=Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles),"System Companion");
        ProtectDirectory(root);root=Path.Combine(root,"Sensors");ProtectDirectory(root);
        string destination=Path.Combine(root,sid);ProtectDirectory(destination);
        CopyProtected(source,destination,"Sensors.exe");
        foreach(string name in Libraries)CopyProtected(source,destination,name);
        dynamic scheduler=Activator.CreateInstance(Type.GetTypeFromProgID("Schedule.Service"));scheduler.Connect();
        dynamic folder=scheduler.GetFolder("\\");dynamic task=scheduler.NewTask(0);
        task.RegistrationInfo.Description="Read-only CPU temperatures for System Companion. Starts on demand; exits after twelve seconds without a monitor request.";
        task.Principal.UserId="SYSTEM";task.Principal.LogonType=5;task.Principal.RunLevel=1;
        task.Settings.Enabled=true;task.Settings.AllowDemandStart=true;task.Settings.DisallowStartIfOnBatteries=false;task.Settings.StopIfGoingOnBatteries=false;
        task.Settings.MultipleInstances=2;task.Settings.ExecutionTimeLimit="PT0S";
        dynamic action=task.Actions.Create(0);action.Path=Path.Combine(destination,"Sensors.exe");action.Arguments="--persistent --sid "+sid;action.WorkingDirectory=destination;
        string security="O:BAG:BAD:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGX;;;"+sid+")";
        dynamic registered=folder.RegisterTaskDefinition(TaskPrefix+sid,task,22,"SYSTEM",null,5,security);
        registered.Run(null);SetupLog("CPU helper registered successfully");return 0;
    }
    static void CopyProtected(string source,string destination,string name) {
        string target=Path.Combine(destination,name);
        if(File.Exists(target)&&(File.GetAttributes(target)&FileAttributes.ReparsePoint)!=0)throw new IOException("Sensor file is a reparse point");
        File.Copy(Path.Combine(source,name),target,true);
        var acl=new FileSecurity();acl.SetAccessRuleProtection(false,false);acl.SetOwner(new SecurityIdentifier(WellKnownSidType.BuiltinAdministratorsSid,null));
        File.SetAccessControl(target,acl);
    }
    sealed class CpuSnapshot {
        public volatile string Json;
        public volatile bool Stop;
        public void Start() {
            Json="{\"time\":"+DateTimeOffset.UtcNow.ToUnixTimeSeconds()+",\"sensor_status\":\"CPU sensors warming up\"}";
            var thread=new Thread(delegate() {
                while(!Stop) {
                    Computer computer=null;
                    try {
                        computer=new Computer{IsCpuEnabled=true};computer.Open();
                        while(!Stop) {
                            var values=new Dictionary<string,string>();
                            foreach(IHardware hardware in computer.Hardware)Visit(hardware,values);
                            values["time"]=DateTimeOffset.UtcNow.ToUnixTimeSeconds().ToString(CultureInfo.InvariantCulture);
                            if(!values.ContainsKey("cpu_temp"))values["sensor_status"]=Encode("CPU temperature unavailable from the existing driver");
                            var parts=new List<string>();foreach(var pair in values)parts.Add(Encode(pair.Key)+":"+pair.Value);
                            Json="{"+string.Join(",",parts.ToArray())+"}";
                            for(int i=0;i<20&&!Stop;i++)Thread.Sleep(100);
                        }
                    } catch(Exception error) {
                        Json="{\"time\":"+DateTimeOffset.UtcNow.ToUnixTimeSeconds()+",\"sensor_status\":"+Encode("CPU sensor reader: "+error.Message)+"}";
                    } finally {if(computer!=null)try{computer.Close();}catch{}}
                    // Retry a failed driver on a bounded schedule, independently of IPC.
                    for(int i=0;i<100&&!Stop;i++)Thread.Sleep(100);
                }
            });
            thread.IsBackground=true;thread.Name="Monitor CPU sampling";thread.Start();
        }
    }
    static int Persistent(string sid) {
        using(var identity=WindowsIdentity.GetCurrent())if(identity.User.Value!=sid&&!identity.User.IsWellKnown(WellKnownSidType.LocalSystemSid))return 3;
        var security=new PipeSecurity();security.SetAccessRuleProtection(true,false);
        security.AddAccessRule(new PipeAccessRule(new SecurityIdentifier(sid),PipeAccessRights.ReadWrite,AccessControlType.Allow));
        security.AddAccessRule(new PipeAccessRule(new SecurityIdentifier(WellKnownSidType.LocalSystemSid,null),PipeAccessRights.FullControl,AccessControlType.Allow));
        var snapshot=new CpuSnapshot();snapshot.Start();
        try {
            while(true) {
                try {
                    using(var pipe=new NamedPipeServerStream(TaskPrefix+sid,PipeDirection.InOut,1,PipeTransmissionMode.Message,PipeOptions.Asynchronous,128,4096,security)) {
                        var wait=pipe.BeginWaitForConnection(null,null);
                        using(var handle=wait.AsyncWaitHandle){if(!handle.WaitOne(12000))return 0;pipe.EndWaitForConnection(wait);}
                        byte[] command=new byte[4];var read=pipe.BeginRead(command,0,4,null,null);
                        int length;using(var handle=read.AsyncWaitHandle){if(!handle.WaitOne(1500))continue;length=pipe.EndRead(read);}
                        if(length!=4||Encoding.ASCII.GetString(command)!="READ")continue;
                        byte[] output=Encoding.UTF8.GetBytes(snapshot.Json);pipe.Write(output,0,output.Length);pipe.Flush();
                    }
                } catch(IOException) { /* A timed-out client must not restart slow sensor initialization. */ }
            }
        } finally {snapshot.Stop=true;}
    }
    static string Encode(string value) { return "\""+value.Replace("\\","\\\\").Replace("\"","\\\"").Replace("\r"," ").Replace("\n"," ")+"\""; }
    static void Visit(IHardware h,Dictionary<string,string> values) {
        h.Update();
        foreach(ISensor sensor in h.Sensors) {
            if(!sensor.Value.HasValue) continue;
            if(sensor.Value.Value<=0 || float.IsNaN(sensor.Value.Value) || float.IsInfinity(sensor.Value.Value)) continue;
            string name=sensor.Name.ToLowerInvariant();string key=null;
            if(h.HardwareType==HardwareType.Cpu) {
                if(sensor.SensorType==SensorType.Temperature&&(name.Contains("package")||name.Contains("tctl")||name.Contains("tdie"))) key="cpu_temp";
                else if(sensor.SensorType==SensorType.Power&&name.Contains("package")) key="cpu_power";
                else if(sensor.SensorType==SensorType.Clock&&name.Contains("core #1")) key="cpu_clock";
                else if(sensor.SensorType==SensorType.Voltage&&name.Contains("core")) key="cpu_voltage";
            }
            if(key!=null) values[key]=sensor.Value.Value.ToString(CultureInfo.InvariantCulture);
        }
        foreach(IHardware child in h.SubHardware) Visit(child,values);
    }
    static int Main(string[] args) {
        string sid=null;bool persistent=false,install=false;
        for(int i=0;i<args.Length;i++){if(args[i]=="--sid"&&i+1<args.Length)sid=args[++i];else if(args[i]=="--persistent")persistent=true;else if(args[i]=="--install-persistent")install=true;}
        string filename=Path.GetFileNameWithoutExtension(Assembly.GetExecutingAssembly().Location);
        if(filename=="Monitor CPU Setup") {sid=Path.GetFileName(Path.GetDirectoryName(Assembly.GetExecutingAssembly().Location));install=true;}
        if(install||persistent){try {if(sid==null)return 2;return install?InstallPersistent(sid):Persistent(sid);}catch(Exception error){if(install)SetupLog(error.GetType().Name+": "+error.Message);if(Array.IndexOf(args,"--diagnostic")>=0)Console.Error.WriteLine(error);return 1;}}
        if(args.Length==0)return 2;
        return LegacyReader(args);
    }
    static int LegacyReader(string[] args) {
        string output=null;int parent=0;bool once=false;
        for(int i=0;i<args.Length;i++) {if(args[i]=="--once") once=true;else if(args[i]=="--out"&&i+1<args.Length) output=args[++i];else if(args[i]=="--parent"&&i+1<args.Length) parent=int.Parse(args[++i]);}
        Computer computer=new Computer {IsCpuEnabled=true};
        try {
            computer.Open();
            int unavailable=0;
            do {
                if(parent>0) {try {using(var owner=Process.GetProcessById(parent)) {if(owner.HasExited) break;}}catch {break;}}
                var values=new Dictionary<string,string>();values["time"]=DateTimeOffset.UtcNow.ToUnixTimeSeconds().ToString();
                foreach(IHardware hardware in computer.Hardware) Visit(hardware,values);
                if(!values.ContainsKey("cpu_temp")) values["sensor_status"]=Encode("CPU temperature unavailable. Select Enable CPU temperatures for optional sensor access.");
                var parts=new List<string>();foreach(var pair in values) parts.Add(Encode(pair.Key)+":"+pair.Value);
                string json="{"+string.Join(",",parts.ToArray())+"}";
                if(once) {Console.WriteLine(json);break;}
                if(output!=null) File.WriteAllText(output,json,new UTF8Encoding(false));
                // A reader without CPU sensor access should not keep polling.
                if(!values.ContainsKey("cpu_temp")) {if(++unavailable>=3) break;} else unavailable=0;
                Thread.Sleep(2000);
            }while(true);
            return 0;
        }catch(Exception error) {if(once) Console.WriteLine("{\"sensor_status\":"+Encode(error.Message)+"}");return 1;}
        finally {computer.Close();}
    }
}

