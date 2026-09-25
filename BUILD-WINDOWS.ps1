[CmdletBinding()]
param(
    [ValidatePattern('^\d+\.\d+\.\d+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$')][string]$Version = '0.5.33',
    [string]$Revision = 'source',
    [switch]$Test
)
$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    foreach ($tool in @('go', 'cargo', 'rustc', 'gcc', 'g++', 'windres')) {
        if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
            throw "Install $tool and add it to PATH; see BUILDING.txt."
        }
    }
    $env:CGO_ENABLED = '1'
    # Keep local source and dependency paths out of Rust panic locations.
    $cargoRoot = if ($env:CARGO_HOME) { [System.IO.Path]::GetFullPath($env:CARGO_HOME) } else { Join-Path ([Environment]::GetFolderPath('UserProfile')) '.cargo' }
    $rustPathMappings = @(
        "--remap-path-prefix=$PSScriptRoot=source"
        "--remap-path-prefix=$($PSScriptRoot.Replace('\', '/'))=source"
        "--remap-path-prefix=$cargoRoot=cargo"
        "--remap-path-prefix=$($cargoRoot.Replace('\', '/'))=cargo"
    )
    $env:CARGO_ENCODED_RUSTFLAGS = $rustPathMappings -join [char]0x1f
    if (-not $env:CC) { $env:CC = (Get-Command gcc).Source }
    if (-not $env:CXX) { $env:CXX = (Get-Command g++).Source }
    $moduleMode = 'mod'
    if (Test-Path -LiteralPath 'vendor/modules.txt') { $moduleMode = 'vendor' }
    $previewArgs = @()
    if (Test-Path -LiteralPath 'preview-dependencies') { $previewArgs += @('-cache', 'preview-dependencies') }
    & go run "-mod=$moduleMode" ./cmd/bundlepreviews @previewArgs
    if ($LASTEXITCODE -ne 0) { throw 'Preview runtime packaging failed.' }
    Push-Location third_party/sdmmparser/src
    try {
        $cargoArgs = @('build', '--release', '--locked')
        if (Test-Path -LiteralPath 'vendor') { $cargoArgs += '--offline' }
        & cargo @cargoArgs
        if ($LASTEXITCODE -ne 0) { throw 'Rust parser build failed.' }
    } finally { Pop-Location }
    if ($Test) {
        & go test "-mod=$moduleMode" -tags bundled_previews ./...
        if ($LASTEXITCODE -ne 0) { throw 'Tests failed.' }
    }
    $sharedFlags = "-s -w -X sdmm/internal/env.Version=$Version -X sdmm/internal/env.Revision=$Revision -extldflags=-static"
    [void][System.IO.Directory]::CreateDirectory((Join-Path $PSScriptRoot 'dst'))
    $resourceText = [System.IO.File]::ReadAllText((Join-Path $PSScriptRoot 'distribution/Voidworks.rc.in'))
    $resourceVersion = ($Version -split '-', 2)[0]
    $resourceText = $resourceText.Replace('@VERSION@', $Version).Replace('@VERSION_NUMBERS@', (($resourceVersion -split '\.') -join ',') + ',0')
    [System.IO.File]::WriteAllText((Join-Path $PSScriptRoot 'dst/version.rc'), $resourceText)
    & windres -i dst/version.rc -o resource_windows_amd64.syso -O coff --target=pe-x86-64
    if ($LASTEXITCODE -ne 0) { throw 'Windows resource build failed.' }
    try {
        & go build "-mod=$moduleMode" -tags bundled_previews -buildvcs=false -trimpath "-ldflags=$sharedFlags -H windowsgui" -o dst/Voidworks.exe .
        if ($LASTEXITCODE -ne 0) { throw 'Editor build failed.' }
    } finally { Remove-Item -LiteralPath (Join-Path $PSScriptRoot 'resource_windows_amd64.syso') -ErrorAction SilentlyContinue }
    & go build "-mod=$moduleMode" -buildvcs=false -trimpath "-ldflags=$sharedFlags" -o dst/shipcheck.exe ./cmd/shipcheck
    if ($LASTEXITCODE -ne 0) { throw 'Checker build failed.' }
    Write-Host 'Built dst/Voidworks.exe and dst/shipcheck.exe.'
} finally { Pop-Location }
