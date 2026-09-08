$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$env:GOPATH = Join-Path $projectRoot '.tools\gopath'
$env:GOCACHE = Join-Path $projectRoot '.tools\gocache'
$env:GOTOOLCHAIN = 'local'
$env:CGO_ENABLED = '0'
$env:Path = (Join-Path $projectRoot '.tools\go\bin') + ';' + $env:Path
$env:npm_config_cache = Join-Path $projectRoot '.tools\npm-cache'
