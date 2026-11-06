# Windows PowerShell 5.1 / built-in .NET only. No Python or build tools.
$ErrorActionPreference = 'Stop'
$manifest = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'download-manifest.json') -Raw | ConvertFrom-Json
if ($manifest.image -notmatch '^shomer-live-[A-Za-z0-9.-]+-amd64\.iso$') { throw 'Invalid image filename' }
$output = Join-Path $PSScriptRoot $manifest.image
if (Test-Path -LiteralPath $output) { throw 'Output already exists; preserve it or choose a fresh download directory' }
$partial = $output + '.partial'
$stream = [IO.File]::Open($partial, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write)
try {
    foreach ($part in $manifest.parts) {
        if ($part.name -notmatch ('^' + [regex]::Escape($manifest.image) + '\.part-[0-9]{3}$')) { throw 'Invalid part filename' }
        $path = Join-Path $PSScriptRoot $part.name
        if ((Get-Item -LiteralPath $path).Length -ne [long]$part.size) { throw "Size mismatch: $path" }
        if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -ne $part.sha256) { throw "Checksum mismatch: $path" }
        Write-Host ('Joining ' + $part.name)
        $source = [IO.File]::OpenRead($path)
        try { $source.CopyTo($stream) } finally { $source.Dispose() }
    }
} finally { $stream.Dispose() }
if ((Get-Item -LiteralPath $partial).Length -ne [long]$manifest.size) { throw 'Reconstructed ISO size mismatch' }
if ((Get-FileHash -LiteralPath $partial -Algorithm SHA256).Hash -ne $manifest.sha256) { throw 'Reconstructed ISO SHA-256 mismatch' }
Move-Item -LiteralPath $partial -Destination $output
Write-Host ('Verified ISO ready for Rufus: ' + $output)
