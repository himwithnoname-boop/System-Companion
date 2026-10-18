param(
    [string]$Python='python',
    [string]$Clang='',
    [string]$LzmaSdk=''
)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
if(!$LzmaSdk){$LzmaSdk=Join-Path $root '.local\lzma-sdk'}
if(!$Clang){
    $compiler=Get-Command clang.exe -ErrorAction SilentlyContinue
    if($compiler){$Clang=$compiler.Source}

}
if(!$Clang -or !(Test-Path -LiteralPath $Clang)){throw 'Pass -Clang with an existing LLVM compiler; portable Linux decoders require cross-compilation.'}
if(!(Test-Path -LiteralPath (Join-Path $LzmaSdk 'bin\x64\7zr.exe'))){throw 'LZMA SDK build tools missing. Provide -LzmaSdk; see https://www.7-zip.org/sdk.html. No tools are downloaded by this build.'}
$names=@('XzDec.c','Xz.c','XzCrc64.c','XzCrc64Opt.c','LzmaDec.c','Lzma2Dec.c','7zCrc.c','7zCrcOpt.c','Sha256.c','Sha256Opt.c','CpuArch.c','Bra.c','Bra86.c','BraIA64.c','Delta.c','Alloc.c')
$sources=@((Join-Path $root 'tools\portable-xz.c'))+@($names | ForEach-Object {Join-Path $root ('tools\vendor\lzma\'+$_)})
New-Item -ItemType Directory -Force -Path (Join-Path $root '.local\portable-tools') | Out-Null
foreach($target in @(@('amd64','x86_64-linux-android21'),@('arm64','aarch64-linux-android21'))){
    $output=Join-Path $root ('.local\portable-tools\xz-'+$target[0])
    & $Clang ('--target='+$target[1]) -Os -s -static -nostdlib -fno-pie -ffreestanding -fno-builtin -fno-stack-protector -ffunction-sections -fdata-sections '-Wl,--gc-sections' '-Wl,-e,_start' '-Wl,-z,max-page-size=4096' -DZ7_ST -DZ7_CRC_NUM_TABLES=1 ('-I'+(Join-Path $root 'tools\vendor\lzma')) @sources -o $output
    if($LASTEXITCODE -ne 0){throw ('Linux decoder build failed: '+$target[0])}
}
& $Python (Join-Path $root 'tools\package-device.py') --sdk $LzmaSdk
if($LASTEXITCODE -ne 0){throw 'Portable device packaging failed'}
