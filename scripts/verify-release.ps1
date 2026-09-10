[CmdletBinding()]
param([Parameter(Mandatory)][string]$Archive, [switch]$RequireSigned)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$zipPath = (Resolve-Path -LiteralPath $Archive).Path
$evidence = Join-Path $root ('artifacts/release-verification/' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $evidence | Out-Null
$hash = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
$checksum = (Get-Content -LiteralPath ($zipPath + '.sha256') -Raw).Trim().Split(' ')[0]
if ($hash -ne $checksum) { throw 'Archive checksum does not match.' }
Expand-Archive -LiteralPath $zipPath -DestinationPath (Join-Path $evidence 'extracted')
$package = Get-ChildItem -LiteralPath (Join-Path $evidence 'extracted') -Directory
if (@($package).Count -ne 1) { throw 'Expected exactly one package root.' }
$package = $package.FullName
$verified = 0
$listed = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
foreach ($line in Get-Content -LiteralPath (Join-Path $package 'SHA256SUMS.txt')) {
    if ($line -notmatch '^([a-f0-9]{64})  (.+)$') { throw 'Malformed package checksum list.' }
    $expected, $relative = $Matches[1], $Matches[2]
    $file = [IO.Path]::GetFullPath((Join-Path $package $relative))
    if (-not $file.StartsWith($package + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) { throw 'Checksum path escapes package.' }
    if ((Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected) { throw "Package checksum failed: $relative" }
    if (-not $listed.Add($file)) { throw 'Duplicate package checksum entry.' }
    $verified++
}
foreach ($file in Get-ChildItem -LiteralPath $package -File -Recurse) {
    if ($file.Name -eq 'SHA256SUMS.txt' -and $file.DirectoryName -eq $package) { continue }
    if (-not $listed.Contains($file.FullName)) { throw "Package contains an unchecked file: $($file.Name)" }
}
foreach ($required in @('ColorNinja.exe', 'colorninja-cli.exe', 'BUILD-INFO.json', 'START-HERE.txt', 'LICENSE')) {
    if (-not $listed.Contains((Join-Path $package $required))) { throw "Package is missing required file: $required" }
}
$info = Get-Content -LiteralPath (Join-Path $package 'BUILD-INFO.json') -Raw | ConvertFrom-Json
$cli = Join-Path $package 'colorninja-cli.exe'
foreach ($file in @($cli, (Join-Path $package 'ColorNinja.exe'))) {
    if ($RequireSigned -and (Get-AuthenticodeSignature -LiteralPath $file).Status -ne 'Valid') { throw "A trusted signature is required: $file" }
}
if ($RequireSigned -and $info.sourceDirty) { throw 'A production release requires clean-source provenance.' }
$inputImage = Join-Path $package 'internal/studio/assets/color-pop-poppy.png'
$library = Join-Path $root 'internal/engine/testdata/library.json'
$cases = @()
foreach ($model in @('frontlit', 'backlit')) {
    foreach ($mode in @('color-match', 'ordered', 'color-pop')) {
        $name = "$mode-$model"
        $png = Join-Path $evidence "$name.png"
        $report = Join-Path $evidence "$name.json"
        $hfp = Join-Path $evidence "$name.hfp"
        $arguments = @($inputImage, '-o', $png, '--hueforge-library', $library, '--hueforge-stack', '--colors', '4', '--hueforge-max-depth', '1.44', '--hueforge-max-runs', '8', '--palette-json', $report, '--hueforge-project', $hfp, '--colorninja-project', (Join-Path $evidence "$name.colorninja"), '--quiet')
        if ($mode -eq 'color-pop') { $arguments += '--color-pop' }
        elseif ($mode -eq 'ordered') { $arguments += @('--color-order', 'black,green,red,white', '--color-order-weight', '75') }
        if ($model -eq 'backlit') { $arguments += @('--hueforge-optical-model', 'hueforge-0.9.4.3-backlit-v1') }
        $arguments += @('--hueforge-height-map', (Join-Path $evidence "$name.layers.png"))
        $timer = [Diagnostics.Stopwatch]::StartNew()
        & $cli @arguments
        if ($LASTEXITCODE -ne 0) { throw "Packaged CLI failed: $name" }
        $timer.Stop()
        $result = Get-Content -LiteralPath $report -Raw | ConvertFrom-Json
        $project = Get-Content -LiteralPath $hfp -Raw | ConvertFrom-Json
        if ($result.result.stack.uniqueFilaments -gt 4 -or @($result.result.stack.runs).Count -gt 8 -or $result.result.stack.plannedDepth -gt 1.440001) { throw "Stack constraints failed: $name" }
        if ($project.luminance_method -ne 6 -or $project.lighting_visualizer -ne [int]($model -eq 'backlit')) { throw "HFP mode failed: $name" }
        if ($result.result.heightMap) { throw "Paused height workflow is active: $name" }
        if ($mode -eq 'ordered' -and ($result.result.stack.colorOrder.requested -ne 'black,green,red,white' -or $result.result.stack.colorOrder.weight -ne 75)) { throw "Color order was lost: $name" }
        $cases += [ordered]@{name=$name;seconds=[Math]::Round($timer.Elapsed.TotalSeconds,3);rgbaSHA256=$result.result.rgbaSHA256;colors=$result.result.uniqueColors;spools=$result.result.stack.uniqueFilaments;runs=@($result.result.stack.runs).Count;depth=$result.result.stack.plannedDepth;quality=$result.result.quality}
    }
}
foreach ($mode in @('standard','combo','max-channel','scaled-max-channel','color-aware')) {
    & $cli $inputImage -o (Join-Path $evidence "disabled-$mode.png") --hueforge-library $library --hueforge-stack --height-mode $mode --quiet 2> (Join-Path $evidence "disabled-$mode.txt")
    if ($LASTEXITCODE -eq 0 -or (Test-Path -LiteralPath (Join-Path $evidence "disabled-$mode.png"))) { throw "Disabled workflow ran: $mode" }
    if ((Get-Content -LiteralPath (Join-Path $evidence "disabled-$mode.txt") -Raw) -notmatch 'temporarily disabled') { throw "Unexpected disabled-mode error: $mode" }
}
# Prove the extracted CLI refuses accidental overwrites.
& $cli $inputImage -o (Join-Path $evidence 'color-match-frontlit.png') --quiet 2> (Join-Path $evidence 'overwrite-refusal.txt')
if ($LASTEXITCODE -eq 0) { throw 'Packaged CLI overwrote an existing output without --force.' }
$record = [ordered]@{version=$info.version;archiveSHA256=$hash;verifiedFiles=$verified;sourceCommit=$info.sourceCommit;sourceDirty=$info.sourceDirty;signing=$info.signing;cases=$cases;overwriteRefusal='passed';physicalPrintAcceptance='not performed';nativeDialogAcceptance='not performed'}
$record | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath (Join-Path $evidence 'verification.json') -Encoding utf8
Write-Host "Package verified: $verified files, $($cases.Count) workflows. Evidence: $evidence/verification.json"
# Negative checks intentionally leave the CLI's exit code nonzero. Report the
# verifier's success explicitly so CI does not mistake an expected refusal for failure.
exit 0
