[CmdletBinding()]
param(
    [Parameter(Mandatory)][string[]]$Path,
    [Parameter(Mandatory)][ValidatePattern('^[a-fA-F0-9]{40}$')][string]$CertificateThumbprint,
    [string]$SignToolPath = 'signtool.exe',
    [string]$TimestampUrl = 'http://timestamp.digicert.com'
)
$ErrorActionPreference = 'Stop'
$signTool = Get-Command $SignToolPath -ErrorAction Stop
$timestamp = [Uri]$TimestampUrl
if ($timestamp.Scheme -notin @('http', 'https')) { throw 'Timestamp server must be an HTTP or HTTPS URL.' }
foreach ($file in $Path) {
    $resolved = (Resolve-Path -LiteralPath $file).Path
    & $signTool.Source sign /sha1 $CertificateThumbprint /fd SHA256 /tr $TimestampUrl /td SHA256 $resolved
    if ($LASTEXITCODE -ne 0) { throw "Signing failed: $resolved" }
    & $signTool.Source verify /pa /all $resolved
    if ($LASTEXITCODE -ne 0) { throw "Signature verification failed: $resolved" }
    $signature = Get-AuthenticodeSignature -LiteralPath $resolved
    if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Thumbprint -ne $CertificateThumbprint -or -not $signature.TimeStamperCertificate) {
        throw "Signature identity, trust, or timestamp verification failed: $resolved"
    }
}
