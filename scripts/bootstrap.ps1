# Installs project-local, pinned build tools. No system Go/npm installation changes.
[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
Push-Location $projectRoot
try {
    $nodeCommand = Get-Command node -ErrorAction SilentlyContinue
    if (-not $nodeCommand) { throw 'Install Node.js 24 LTS from https://nodejs.org, reopen PowerShell, and run this script again.' }
    $nodeMajor = [int]((& node --version).TrimStart('v').Split('.')[0])
    if ($nodeMajor -lt 24) { throw 'Node.js 24 or newer is required by the pinned npm toolchain.' }
    New-Item -ItemType Directory -Force -Path '.tools' | Out-Null
    $goVersion = 'go1.27.1'
    $goArchive = Join-Path $projectRoot '.tools\go1.27.1.windows-amd64.zip'
    $goSHA256 = 'A3911B5E0E1B1053F25ED0675F4C1C6AAD1E2BFCF253DF2B9BE4CAABD2EDD95D'
    if (-not (Test-Path -LiteralPath '.tools\go\bin\go.exe')) {
        if (-not (Test-Path -LiteralPath $goArchive)) {
            Invoke-WebRequest -Uri "https://go.dev/dl/$goVersion.windows-amd64.zip" -OutFile $goArchive
        }
        if ((Get-FileHash -LiteralPath $goArchive -Algorithm SHA256).Hash -ne $goSHA256) { throw 'Go archive checksum mismatch.' }
        Expand-Archive -LiteralPath $goArchive -DestinationPath '.tools' -Force
    }
    . .\scripts\env.ps1
    if ((& go version) -notmatch 'go1\.27\.1 windows/amd64') { throw 'The project-local Go toolchain does not match Go 1.27.1 for Windows x64.' }
    if (-not (Test-Path -LiteralPath '.tools\npm\package\bin\npm-cli.js')) {
        $npmMetadata = Invoke-RestMethod -Uri 'https://registry.npmjs.org/npm/12.0.2'
        $npmArchive = Join-Path $projectRoot '.tools\npm.tgz'
        Invoke-WebRequest -Uri 'https://registry.npmjs.org/npm/-/npm-12.0.2.tgz' -OutFile $npmArchive
        $hasher = [Security.Cryptography.SHA512]::Create()
        try { $integrity = 'sha512-' + [Convert]::ToBase64String($hasher.ComputeHash([IO.File]::ReadAllBytes($npmArchive))) }
        finally { $hasher.Dispose() }
        if ($integrity -ne $npmMetadata.dist.integrity) { throw 'npm archive integrity mismatch.' }
        New-Item -ItemType Directory -Force -Path '.tools\npm' | Out-Null
        & tar -xzf $npmArchive -C '.tools\npm'
        if ($LASTEXITCODE -ne 0) { throw 'npm archive extraction failed.' }
    }
    if ((& node .tools\npm\package\bin\npm-cli.js --version) -ne '12.0.2') { throw 'The project-local npm toolchain does not match 12.0.2.' }
    & go mod download
    if ($LASTEXITCODE -ne 0) { throw 'Go dependency download failed.' }
    if (-not (Test-Path -LiteralPath '.tools\gopath\bin\wails.exe')) {
        & go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
        if ($LASTEXITCODE -ne 0) { throw 'Wails installation failed.' }
    }
    if (((& .\.tools\gopath\bin\wails.exe version) -join "`n") -notmatch 'v2\.15\.0') { throw 'The project-local Wails version does not match 2.15.0.' }
    & node .tools\npm\package\bin\npm-cli.js ci --prefix frontend
    if ($LASTEXITCODE -ne 0) { throw 'Frontend dependency installation failed.' }
    Write-Host 'Build tools are ready. Run .\scripts\build.ps1.'
} finally { Pop-Location }
