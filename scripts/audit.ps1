[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
try {
    . ./scripts/env.ps1
    $output = Join-Path $root 'artifacts/dependency-audit'
    New-Item -ItemType Directory -Force -Path $output | Out-Null
    $npm = & node .tools/npm/package/bin/npm-cli.js audit --prefix frontend --json
    $npmExit = $LASTEXITCODE
    $npm | Set-Content -LiteralPath (Join-Path $output 'npm.json') -Encoding utf8
    & go install golang.org/x/vuln/cmd/govulncheck@v1.8.0
    if ($LASTEXITCODE -ne 0) { throw 'Could not install pinned Go vulnerability scanner.' }
    $vulnerabilities = & ./.tools/gopath/bin/govulncheck.exe -json -tags 'desktop,production,wv2runtime.download' . ./cmd/colorninja-cli
    $goExit = $LASTEXITCODE
    $vulnerabilities | Set-Content -LiteralPath (Join-Path $output 'go.jsonl') -Encoding utf8
    if ($npmExit -ne 0 -or $goExit -ne 0) { throw 'Dependency audit failed. Review artifacts/dependency-audit.' }
    Write-Host 'npm audit and govulncheck passed. Evidence: artifacts/dependency-audit'
} finally { Pop-Location }
