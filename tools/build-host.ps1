param([string]$Python='python',[string]$Clang='',[string]$LzmaSdk='',[string]$Go=(Join-Path (Split-Path -Parent $PSScriptRoot) '.local\toolchains\go\bin\go.exe'))
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
if(!(Test-Path -LiteralPath $Go)) {
    $availableGo=Get-Command go -ErrorAction SilentlyContinue
    if(!$availableGo){throw 'Install Go 1.26+ or pass -Go with its executable.'}
    $Go=$availableGo.Source
}

$versionMatch=[regex]::Match((Get-Content -LiteralPath (Join-Path $root 'host\main.go') -Raw),'var version = "([^"]+)"')
if(!$versionMatch.Success){throw 'Host version missing'}
$version=$versionMatch.Groups[1].Value
$noticeDestination=Join-Path $root 'dist\SYSTEM_MONITOR\Licenses'
New-Item -ItemType Directory -Force -Path $noticeDestination | Out-Null
Get-ChildItem -LiteralPath $noticeDestination -File | ForEach-Object {$_.IsReadOnly=$false}
Get-ChildItem -LiteralPath (Join-Path $root 'licenses') -File | Where-Object {$_.Name -notin @('DiskInfoToolkit-MPL-2.0.txt','RAMSPDToolkit-MPL-2.0.txt')} | Copy-Item -Destination $noticeDestination -Force
Copy-Item -LiteralPath (Join-Path $root 'LICENSE') -Destination (Join-Path $noticeDestination 'SystemCompanion-MIT.txt') -Force
Copy-Item -LiteralPath (Join-Path $root 'THIRD_PARTY_NOTICES.md') -Destination (Join-Path $noticeDestination 'THIRD-PARTY-NOTICES.txt') -Force
$goRoot=& $Go env GOROOT
if(Test-Path (Join-Path $goRoot 'LICENSE')){Copy-Item -LiteralPath (Join-Path $goRoot 'LICENSE') -Destination (Join-Path $noticeDestination 'Go-LICENSE.txt') -Force}
$env:CGO_ENABLED='0' 
Push-Location (Join-Path $root 'host')
try {
    $sensorSource=Join-Path $root '.local\lhm'
    $sensorDestination=Join-Path $root 'dist\SYSTEM_MONITOR\Windows\Sensors'
    if(Test-Path (Join-Path $sensorSource 'LibreHardwareMonitorLib.dll')) {
        New-Item -ItemType Directory -Force -Path $sensorDestination | Out-Null
        $libraries=@('LibreHardwareMonitorLib','HidSharp','DiskInfoToolkit','RAMSPDToolkit-NDD','Microsoft.Bcl.AsyncInterfaces','Microsoft.Bcl.HashCode','System.Buffers','System.Memory','System.Numerics.Vectors','System.Runtime.CompilerServices.Unsafe','System.Threading.Tasks.Extensions','System.IO.Pipelines')
        foreach($library in $libraries) {Copy-Item -LiteralPath (Join-Path $sensorSource ($library+'.dll')) -Destination $sensorDestination}
        & (Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe') /nologo /target:winexe /optimize+ /reference:Microsoft.CSharp.dll /reference:System.Management.dll ("/out:"+(Join-Path $sensorDestination 'Sensors.exe')) ("/reference:"+(Join-Path $sensorSource 'LibreHardwareMonitorLib.dll')) (Join-Path $root 'host\Sensors.cs')
        if($LASTEXITCODE -ne 0){throw 'Optional Windows sensor helper build failed'}
    }
    $targets=@(@('windows','amd64','Windows\Monitor.exe'),@('windows','arm64','Windows-arm64\Monitor.exe'),@('linux','amd64','Linux\Monitor'),@('linux','arm64','Linux-arm64\Monitor'),@('darwin','arm64','macOS\Monitor.app\Contents\MacOS\Monitor'))
    foreach($target in $targets) {
        $env:GOOS=$target[0];$env:GOARCH=$target[1]
        $output=Join-Path $root ('dist\SYSTEM_MONITOR\'+$target[2]);New-Item -ItemType Directory -Force -Path (Split-Path -Parent $output) | Out-Null
        $flags='-s -w';if($target[0] -eq 'windows'){$flags+=' -H windowsgui'}
        & $Go build -trimpath -ldflags $flags -o $output .
        if($LASTEXITCODE -ne 0){throw "Build failed for $($target[0])/$($target[1])"}
        Write-Output "$($target[0])/$($target[1]): $((Get-Item $output).Length) bytes"
    }
    if(Test-Path -LiteralPath $sensorDestination) {
        $armSensors=Join-Path $root 'dist\SYSTEM_MONITOR\Windows-arm64\Sensors'
        New-Item -ItemType Directory -Force -Path $armSensors | Out-Null
        Get-ChildItem -LiteralPath $sensorDestination -File | Where-Object {$_.Extension -eq '.dll' -or $_.Name -eq 'Sensors.exe'} | Copy-Item -Destination $armSensors
    }
    $env:GOOS='windows';$env:GOARCH='386'
    $selector=Join-Path $root '.local\portable-tools\windows-launcher.exe'
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $selector) | Out-Null
    & $Go build -trimpath -ldflags '-s -w -H windowsgui' -o $selector ./cmd/windows-launcher
    if($LASTEXITCODE -ne 0){throw 'Windows native architecture selector build failed'}
    foreach($platform in @('macOS')) {
        $contents=Join-Path $root "dist\SYSTEM_MONITOR\$platform\Monitor.app\Contents"
        $plist='<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>CFBundleExecutable</key><string>Monitor</string><key>CFBundleIdentifier</key><string>local.systemcompanion.monitor</string><key>CFBundleName</key><string>System Companion</string><key>CFBundleVersion</key><string>1.0.0</string><key>CFBundlePackageType</key><string>APPL</string><key>LSUIElement</key><true/></dict></plist>'
        [IO.File]::WriteAllText((Join-Path $contents 'Info.plist'),$plist.Replace('<string>1.0.0</string>',('<string>'+$version+'</string>')))
    }
    & (Join-Path $PSScriptRoot 'build-portable.ps1') -Python $Python -Clang $Clang -LzmaSdk $LzmaSdk
} finally {Pop-Location;Remove-Item Env:GOOS -ErrorAction SilentlyContinue;Remove-Item Env:GOARCH -ErrorAction SilentlyContinue}
