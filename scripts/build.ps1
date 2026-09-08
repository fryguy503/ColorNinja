[CmdletBinding()]
param([string]$GuiName = 'ColorNinja-Studio.exe')
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
if ([IO.Path]::GetFileName($GuiName) -ne $GuiName -or $GuiName -notmatch '\.exe$') { throw 'GuiName must be a filename ending in .exe.' }
Push-Location $projectRoot
try {
    . .\scripts\env.ps1
    $version = (Get-Content -LiteralPath 'frontend\package.json' -Raw | ConvertFrom-Json).version
    if ($version -notmatch '^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$') { throw 'Invalid application version.' }
    if (-not (Test-Path -LiteralPath '.tools\gopath\bin\wails.exe')) { throw 'Run .\scripts\bootstrap.ps1 first.' }
    & node .tools\npm\package\bin\npm-cli.js run format:check --prefix frontend
    if ($LASTEXITCODE -ne 0) { throw 'Frontend formatting check failed.' }
    & node .tools\npm\package\bin\npm-cli.js test --prefix frontend
    if ($LASTEXITCODE -ne 0) { throw 'Frontend regression tests failed.' }
    & node .tools\npm\package\bin\npm-cli.js run build --prefix frontend
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed.' }
    New-Item -ItemType Directory -Force -Path 'artifacts', 'build\bin' | Out-Null
    $testLog = & go test -json ./...
    $testExit = $LASTEXITCODE
    $testLog | Set-Content -LiteralPath 'artifacts\go-tests.jsonl' -Encoding utf8
    if ($testExit -ne 0) { $testLog | Write-Output; throw 'Go tests failed.' }
    $events = @($testLog | ForEach-Object { $_ | ConvertFrom-Json })
    $passed = @($events | Where-Object { $_.Action -eq 'pass' -and $_.Test -and $_.Test -notmatch '/' }).Count
    Write-Host "$passed Go tests passed. Log: artifacts\go-tests.jsonl"
    & go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }
    & go run ./cmd/artwork
    if ($LASTEXITCODE -ne 0) { throw 'Application artwork generation failed.' }
    & .\.tools\gopath\bin\wails.exe build -skipbindings -s -o $GuiName -trimpath
    if ($LASTEXITCODE -ne 0) { throw 'Desktop build failed. Close the target executable if it is running, or select a different -GuiName.' }
    & go build -trimpath -ldflags '-s -w' -o 'build/bin/colorninja-cli.exe' ./cmd/colorninja-cli
    if ($LASTEXITCODE -ne 0) { throw 'CLI build failed.' }
    $buildInfo = [ordered]@{
        product = 'ColorNinja Studio'; version = $version; platform = 'windows-amd64'
        builtAt = [DateTime]::UtcNow.ToString('o'); go = (& go version)
        node = (& node --version); wails = '2.15.0'; gui = $GuiName
        goTestsPassed = $passed; goVet = 'passed'; typescriptBuild = 'passed'; frontendRegressionTests = 'passed'
        pythonReferenceCases = 5
    }
    if ((Test-Path -LiteralPath '.git') -and (Get-Command git -ErrorAction SilentlyContinue)) {
        $sourceCommit = & git rev-parse HEAD
        if ($LASTEXITCODE -ne 0) { throw 'Could not identify the source commit.' }
        $sourceStatus = & git status --porcelain
        if ($LASTEXITCODE -ne 0) { throw 'Could not identify the source working-tree state.' }
        $buildInfo.sourceCommit = $sourceCommit
        $buildInfo.sourceDirty = [bool]$sourceStatus
    }
    $buildInfo.guiSHA256 = (Get-FileHash -LiteralPath (Join-Path 'build\bin' $GuiName) -Algorithm SHA256).Hash.ToLowerInvariant()
    $buildInfo.cliSHA256 = (Get-FileHash -LiteralPath 'build\bin\colorninja-cli.exe' -Algorithm SHA256).Hash.ToLowerInvariant()
    $buildInfo | ConvertTo-Json | Set-Content -LiteralPath 'build\bin\BUILD-INFO.json' -Encoding utf8
    Get-Item -LiteralPath (Join-Path 'build\bin' $GuiName), 'build\bin\colorninja-cli.exe' | Select-Object FullName, Length
} finally { Pop-Location }
