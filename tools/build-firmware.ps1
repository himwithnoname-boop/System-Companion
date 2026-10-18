param([ValidateSet('build','flash','app-flash','storage-flash','reconfigure')][string]$Action='build',[string]$Port='',[string]$ToolchainRoot='')
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
if($Action -in @('flash','app-flash','storage-flash') -and !$Port){throw 'Pass -Port with the discovered bootloader port.'}
if(!$ToolchainRoot) {
    if(!$env:IDF_PATH){throw 'Run this from an ESP-IDF 5.5.x exported shell, or pass -ToolchainRoot explicitly.'}
    $idfPython=if($env:IDF_PYTHON_ENV_PATH){Join-Path $env:IDF_PYTHON_ENV_PATH 'Scripts\python.exe'}else{'python'}
    $idfAction=if($Action -in @('flash','app-flash','storage-flash')){'build'}else{$Action}
    & $idfPython (Join-Path $env:IDF_PATH 'tools\idf.py') --no-ccache -C (Join-Path $root 'firmware') $idfAction
    if($LASTEXITCODE -ne 0){throw 'ESP-IDF build failed'}
    if($Action -in @('flash','app-flash','storage-flash')) {
        Push-Location (Join-Path $root 'firmware\build')
        try {
            [string[]]$imageArgs=if($Action -eq 'app-flash'){@('@flash_app_args')}elseif($Action -eq 'storage-flash'){@('0x300000','storage.bin')}else{@('@flash_args')}
            & $idfPython -m esptool --chip esp32s3 --port $Port --baud 921600 --before no_reset --after no_reset_stub write_flash @imageArgs
            if($LASTEXITCODE -ne 0){throw 'Flash failed'}
        } finally {Pop-Location}
        & $idfPython (Join-Path $PSScriptRoot 'reset-board.py')
        if($LASTEXITCODE -ne 0){throw 'Board restart failed'}
    }
    return
}
if((Test-Path 'L:\') -or (Test-Path 'T:\')) { throw 'L: or T: is occupied. Wait for the other ESP-IDF build.' }
& subst.exe L: $ToolchainRoot
if($LASTEXITCODE -ne 0) { throw 'Toolchain mapping failed' }
try {
    & subst.exe T: $root
    if($LASTEXITCODE -ne 0) { throw 'Project mapping failed' }
    $env:IDF_PATH='L:\.local\toolchains\esp-idf\esp-idf-v5.5.5'
    $env:IDF_TOOLS_PATH='L:\.local\idf-tools'
    $env:IDF_SKIP_CHECK_SUBMODULES='1'
    $idfPython='L:\.local\idf-tools\python_env\idf5.5_py3.12_env\Scripts\python.exe'
    $env:IDF_PYTHON_ENV_PATH=Split-Path -Parent (Split-Path -Parent $idfPython)
    $exported=& $idfPython "$env:IDF_PATH\tools\idf_tools.py" export --format key-value
    if($LASTEXITCODE -ne 0) { throw ('ESP-IDF export failed: '+($exported -join "`n")) }
    foreach($line in $exported) { if($line -match '^([^=]+)=(.*)$') { [Environment]::SetEnvironmentVariable($Matches[1],$Matches[2].Replace('%PATH%',$env:PATH),'Process') } }
    $idfArgs=@('--no-ccache','-C','T:\firmware')
    $buildAction=if($Action -in @('flash','app-flash','storage-flash')) {'build'} else {$Action}
    & $idfPython "$env:IDF_PATH\tools\idf.py" @idfArgs $buildAction
    if($LASTEXITCODE -ne 0) { throw "Firmware $Action failed" }
    if($Action -in @('flash','app-flash','storage-flash')) {
        Push-Location 'T:\firmware\build'
        try {
            [string[]]$flashArgs=if($Action -eq 'app-flash') {@('@flash_app_args')} elseif($Action -eq 'storage-flash') {@('0x300000','storage.bin')} else {@('@flash_args')}
            & $idfPython -m esptool --chip esp32s3 --port $Port --baud 921600 --before no_reset --after no_reset_stub write_flash @flashArgs
            if($LASTEXITCODE -ne 0) {throw 'Firmware flash failed'}
        } finally {Pop-Location}
        & $idfPython (Join-Path $root 'tools\reset-board.py')
        if($LASTEXITCODE -ne 0) { throw 'Board did not resume after flashing' }
    }
} finally {
    & subst.exe T: /D
    & subst.exe L: /D
}
