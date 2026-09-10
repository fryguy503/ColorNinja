param(
    [Parameter(Mandatory)][string]$InputPath,
    [Parameter(Mandatory)][string]$LibraryPath,
    [Parameter(Mandatory)][string]$OptionsPath,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [Parameter(Mandatory)][string]$CLIPath,
    [string]$AccentFilament = '',
    [switch]$AvoidSilkMetallic,
    [ValidateSet('baseline','boundaries','material','combined','color-refined','combined-refined','combined-auto','accent-top','accent-top-refined','layer-order','layer-order-refined')]
    [string[]]$Variants = @('baseline', 'layer-order', 'layer-order-refined')
)
$ErrorActionPreference = 'Stop'
$taskInput = (Resolve-Path -LiteralPath $InputPath).Path
$taskLibrary = (Resolve-Path -LiteralPath $LibraryPath).Path
$taskCLI = (Resolve-Path -LiteralPath $CLIPath).Path
$taskOptionsText = Get-Content -LiteralPath $OptionsPath -Raw
$taskOutput = [IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Force -Path $taskOutput | Out-Null
if (Get-ChildItem -LiteralPath $taskOutput -Force | Select-Object -First 1) { throw 'Use an empty output directory to preserve prior comparisons.' }
$taskVariants = $Variants
if ($AccentFilament -and 'accent-top-refined' -notin $taskVariants -and 'accent-top' -notin $taskVariants) { $taskVariants += 'accent-top' }
if (@($taskVariants | Select-Object -Unique).Count -ne $taskVariants.Count) { throw 'Comparison variants must be unique.' }
$taskSummary = @()
foreach ($taskVariant in $taskVariants) {
    $taskOptions = $taskOptionsText | ConvertFrom-Json -AsHashtable
    $taskOptions.mode = 'stack'
    if ($taskOptions.colorPop) { $taskOptions.colorPop.enabled = $false }
    $taskHF = $taskOptions.hueforge
    $taskHF.layerPreference = ''
    $taskHF.meshMode = 'color-match'
    $taskFirst = if ($taskHF.firstLayerHeight -gt 0) { $taskHF.firstLayerHeight } else { $taskHF.layerHeight }
    $taskHF.maxDepth = [Math]::Round($taskFirst + [Math]::Floor(($taskHF.maxDepth-$taskFirst)/$taskHF.layerHeight+1e-9)*$taskHF.layerHeight, 8)
    $taskHF.optimizeMaterial = $taskVariant -in @('material','combined','combined-refined','combined-auto','accent-top','accent-top-refined')
    $taskHF.reduceShowThrough = $taskVariant -in @('boundaries','combined','combined-refined','combined-auto','accent-top','accent-top-refined')
    $taskHF.autoDepth = $taskVariant -eq 'combined-auto'
    $taskHF.searchEffort = if ($taskVariant -in @('color-refined','combined-refined','accent-top-refined')) { 'refine' } else { 'preview' }
    if ($taskVariant -in @('layer-order','layer-order-refined')) {
        $taskHF.layerPreference = 'auto'
        $taskHF.surfaceColorTolerance = [Math]::Max(5, $taskHF.surfaceColorTolerance)
        $taskHF.searchEffort = if ($taskVariant -eq 'layer-order-refined') { 'refine' } else { 'preview' }
        $taskHF.autoDepth = [bool](($taskOptionsText | ConvertFrom-Json).hueforge.autoDepth)
    }
    if ($taskVariant -in @('accent-top','accent-top-refined')) {
        if (-not $AccentFilament) { throw 'Accent-top comparisons need an explicit AccentFilament key.' }
        $taskHF.highlightFilament = $AccentFilament
    }
    $taskPrefix = Join-Path $taskOutput $taskVariant
    $taskOptions | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath "$taskPrefix.options.json" -Encoding utf8
    $taskTimer = [Diagnostics.Stopwatch]::StartNew()
    $taskExtraArgs = @()
    if ($AvoidSilkMetallic) { $taskExtraArgs += '--hueforge-avoid-silk-metallic' }
    & $taskCLI $taskInput -o "$taskPrefix.png" --hueforge-library $taskLibrary --options-json "$taskPrefix.options.json" --palette-json "$taskPrefix.json" --hueforge-project "$taskPrefix.hfp" --hueforge-height-map "$taskPrefix.layers.png" --quiet @taskExtraArgs
    if ($LASTEXITCODE -ne 0) { throw "Comparison failed: $taskVariant" }
    $taskTimer.Stop()
    $taskReport = Get-Content -LiteralPath "$taskPrefix.json" -Raw | ConvertFrom-Json
    $taskResult = $taskReport.result
    $taskRow = [ordered]@{
        name = $taskVariant; seconds = [Math]::Round($taskTimer.Elapsed.TotalSeconds,3)
        rmsDeltaE76 = $taskResult.quality.rmsDeltaE76
        volumeCm3 = $taskResult.surfaceView.volumeMm3 / 1000
        meanThicknessMm = $taskResult.surfaceView.meanThicknessMm
        meanBoundaryStepMm = $taskResult.surfaceView.meanJumpMm
        p95BoundaryStepMm = $taskResult.surfaceView.p95JumpMm
        plannedDepthMm = $taskResult.stack.plannedDepth
        spools = $taskResult.stack.uniqueFilaments; runs = $taskResult.stack.runs.Count
        colors = $taskResult.uniqueColors; rgbaSHA256 = $taskResult.rgbaSHA256
        layerPreference = $taskResult.stack.layerPreference
        order = @($taskResult.stack.runs | ForEach-Object { "$($_.filament.brand) $($_.filament.name) ($($_.layers))" })
    }
    $taskSummary += $taskRow
    [pscustomobject]$taskRow | Select-Object name,seconds,rmsDeltaE76,volumeCm3,meanBoundaryStepMm,plannedDepthMm | Format-Table
}
$taskProvenance = [ordered]@{
    sourcePath=$taskInput; sourceSHA256=(Get-FileHash -LiteralPath $taskInput -Algorithm SHA256).Hash
    libraryPath=$taskLibrary; librarySHA256=(Get-FileHash -LiteralPath $taskLibrary -Algorithm SHA256).Hash
    cliSHA256=(Get-FileHash -LiteralPath $taskCLI -Algorithm SHA256).Hash
    optionsSHA256=(Get-FileHash -LiteralPath $OptionsPath -Algorithm SHA256).Hash
    avoidSilkMetallic=[bool]$AvoidSilkMetallic
    variants=$taskSummary
}
$taskProvenance | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath (Join-Path $taskOutput 'summary.json') -Encoding utf8
$taskSummary | ForEach-Object { [pscustomobject]$_ } | Export-Csv -LiteralPath (Join-Path $taskOutput 'summary.csv') -NoTypeInformation
