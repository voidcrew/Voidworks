[CmdletBinding()]
param(
    # Folder holding the extracted release package; its BUILD-INFO.json gives the version.
    [Parameter(Mandatory)][string]$Package,
    [string]$OutputDir = 'dst'
)
$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    $Package = (Resolve-Path -LiteralPath $Package).Path
    [void][System.IO.Directory]::CreateDirectory($OutputDir)
    $OutputDir = (Resolve-Path -LiteralPath $OutputDir).Path
    $version = (Get-Content -LiteralPath (Join-Path $Package 'BUILD-INFO.json') -Raw | ConvertFrom-Json).version
    if ($version -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$') { throw "Invalid package version: $version" }
    $numeric = ($version -split '-', 2)[0] + '.0'

    $compiler = Get-Command iscc -ErrorAction SilentlyContinue
    $compiler = if ($compiler) { $compiler.Source } else {
        @((Join-Path $PSScriptRoot 'dst/toolchain/innosetup/ISCC.exe'), "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe") |
            Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
    }
    if (-not $compiler) { throw 'Install Inno Setup 6 and add ISCC to PATH; see BUILDING.txt.' }

    & $compiler /Q "/DVersion=$version" "/DNumericVersion=$numeric" "/DPackage=$Package" "/DOutput=$OutputDir" distribution/Voidworks.iss
    if ($LASTEXITCODE -ne 0) { throw 'Installer build failed.' }

    $name = "Voidworks-$version-windows-x64-setup.exe"
    $hash = (Get-FileHash -LiteralPath (Join-Path $OutputDir $name) -Algorithm SHA256).Hash.ToLowerInvariant()
    [System.IO.File]::WriteAllText((Join-Path $OutputDir "$name.sha256"), "$hash  $name`n")
    Write-Host "Built $name."
} finally { Pop-Location }
