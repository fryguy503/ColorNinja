[CmdletBinding()]
param([string]$GuiName = 'ColorNinja-Studio.exe', [string]$ReleaseDirectory = 'build\releases')
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
if ([IO.Path]::GetFileName($GuiName) -ne $GuiName) { throw 'GuiName must be a filename.' }
Push-Location $projectRoot
try {
    . .\scripts\env.ps1
    $gui = Join-Path 'build\bin' $GuiName
    $info = Get-Content -LiteralPath 'build\bin\BUILD-INFO.json' -Raw | ConvertFrom-Json
    if ($info.gui -ne $GuiName) { throw 'Build info does not match the requested executable. Run build.ps1 first.' }
    $version = (Get-Content -LiteralPath 'frontend\package.json' -Raw | ConvertFrom-Json).version
    if ($version -notmatch '^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$') { throw 'Invalid application version.' }
    if ($info.version -ne $version) { throw 'Build version is stale. Run build.ps1 first.' }
    if ((Get-FileHash -LiteralPath $gui -Algorithm SHA256).Hash.ToLowerInvariant() -ne $info.guiSHA256) { throw 'Desktop executable does not match build info.' }
    if ((Get-FileHash -LiteralPath 'build\bin\colorninja-cli.exe' -Algorithm SHA256).Hash.ToLowerInvariant() -ne $info.cliSHA256) { throw 'CLI executable does not match build info.' }
    $releaseName = 'ColorNinja-' + $version + '-windows-x64'
    $releaseRoot = [IO.Path]::GetFullPath((Join-Path $projectRoot $ReleaseDirectory))
    if (-not $releaseRoot.StartsWith($projectRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Release directory must stay inside the project.' }
    $release = Join-Path $releaseRoot $releaseName
    $zip = $release + '.zip'
    $archiveChecksum = $zip + '.sha256'
    if ((Test-Path -LiteralPath $release) -or (Test-Path -LiteralPath $zip) -or (Test-Path -LiteralPath $archiveChecksum)) { throw 'This version is already packaged. Use a new version or explicitly archive the previous package first.' }
    New-Item -ItemType Directory -Path $release | Out-Null
    Copy-Item -LiteralPath $gui -Destination (Join-Path $release 'ColorNinja.exe')
    Copy-Item -LiteralPath 'build\bin\colorninja-cli.exe', 'build\bin\BUILD-INFO.json' -Destination $release
    Copy-Item -LiteralPath 'docs\QUICKSTART.txt' -Destination (Join-Path $release 'START-HERE.txt')
    Copy-Item -LiteralPath 'README.md', 'LICENSE' -Destination $release
    Copy-Item -LiteralPath 'docs' -Destination (Join-Path $release 'docs') -Recurse
    New-Item -ItemType Directory -Path (Join-Path $release 'build') | Out-Null
    Copy-Item -LiteralPath 'build\appicon.png' -Destination (Join-Path $release 'build\appicon.png')
    $notice = [Text.StringBuilder]::new()
    [void]$notice.AppendLine('ColorNinja third-party notices. Original license text follows for linked Go modules and bundled JavaScript packages.')
    [void]$notice.AppendLine("`nGo standard library:`n" + [IO.File]::ReadAllText((Join-Path $projectRoot '.tools\go\LICENSE')))
    $modules = & go list -deps -tags 'desktop,production,wv2runtime.download' -f '{{if .Module}}{{.Module.Path}}|{{.Module.Version}}|{{.Module.Dir}}{{end}}' . ./cmd/colorninja-cli
    if ($LASTEXITCODE -ne 0) { throw 'Could not enumerate Go dependencies.' }
    foreach ($row in ($modules | Where-Object { $_ -and $_ -notlike 'colorninja|*' } | Sort-Object -Unique)) {
        $parts = $row.Split('|')
        [void]$notice.AppendLine("`n========== " + $parts[0] + ' ' + $parts[1] + ' ==========')
        $licenses = @(Get-ChildItem -LiteralPath $parts[2] -File | Where-Object { $_.Name -match '^(LICENSE|COPYING|NOTICE)(\.|$)' })
        if ($licenses.Count -eq 0) { throw "Missing license for $($parts[0])" }
        foreach ($license in $licenses) { [void]$notice.AppendLine([IO.File]::ReadAllText($license.FullName)) }
    }
    foreach ($packageName in @('react', 'react-dom', 'scheduler', 'lucide-react')) {
        $packageDir = Join-Path $projectRoot (Join-Path 'frontend\node_modules' $packageName)
        $package = Get-Content -LiteralPath (Join-Path $packageDir 'package.json') -Raw | ConvertFrom-Json
        [void]$notice.AppendLine("`n========== " + $package.name + ' ' + $package.version + ' ==========')
        [void]$notice.AppendLine([IO.File]::ReadAllText((Join-Path $packageDir 'LICENSE')))
    }
    $compiledDependencies = & go version -m $gui 'build\bin\colorninja-cli.exe'
    if ($LASTEXITCODE -ne 0) { throw 'Could not read executable dependency metadata.' }
    foreach ($line in $compiledDependencies) {
        if ($line -match '^\s+dep\s+(\S+)\s+') {
            if (-not $notice.ToString().Contains('========== ' + $Matches[1] + ' ')) { throw "Missing compiled dependency notice: $($Matches[1])" }
        }
    }
    [IO.File]::WriteAllText((Join-Path $release 'THIRD-PARTY-NOTICES.txt'), $notice.ToString())
    $hashes = Get-ChildItem -LiteralPath $release -File -Recurse | Sort-Object FullName | ForEach-Object {
        $relativePath = $_.FullName.Substring($release.Length + 1).Replace('\', '/')
        (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant() + '  ' + $relativePath
    }
    $hashes | Set-Content -LiteralPath (Join-Path $release 'SHA256SUMS.txt') -Encoding ascii
    Compress-Archive -LiteralPath $release -DestinationPath $zip -CompressionLevel Optimal
    $archiveHash = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant()
    ($archiveHash + '  ' + [IO.Path]::GetFileName($zip)) | Set-Content -LiteralPath $archiveChecksum -Encoding ascii
    Get-Item -LiteralPath $zip | Select-Object FullName, Length
    Get-FileHash -LiteralPath $zip -Algorithm SHA256
} finally { Pop-Location }
