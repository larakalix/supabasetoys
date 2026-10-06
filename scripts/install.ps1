param(
  [Parameter(Mandatory=$true)][ValidatePattern('^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$')][string]$Repository,
  [Parameter(Mandatory=$true)][ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+$')][string]$Version,
  [string]$InstallDirectory = (Join-Path $env:LOCALAPPDATA 'SupabaseToys\bin')
)
$ErrorActionPreference = 'Stop'
if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -ne 'X64') { throw 'The Windows v1 CLI supports x64.' }
$asset = "supabase-toys_${Version}_windows_x64.zip"
$base = "https://github.com/$Repository/releases/download/$Version"
$workDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $workDirectory | Out-Null
try {
  Invoke-WebRequest "$base/$asset" -OutFile (Join-Path $workDirectory $asset)
  Invoke-WebRequest "$base/SHA256SUMS" -OutFile (Join-Path $workDirectory 'SHA256SUMS')
  $lines = Get-Content (Join-Path $workDirectory 'SHA256SUMS')
  $match = @($lines | Where-Object { $_ -match ('^[a-fA-F0-9]{64}\s+' + [Regex]::Escape($asset) + '$') })
  if ($match.Count -ne 1) { throw 'Missing or ambiguous release checksum' }
  $expected = ($match[0] -split '\s+')[0]
  $actual = (Get-FileHash (Join-Path $workDirectory $asset) -Algorithm SHA256).Hash
  if ($actual -ne $expected) { throw 'Checksum verification failed; nothing installed' }
  Expand-Archive (Join-Path $workDirectory $asset) -DestinationPath (Join-Path $workDirectory 'extracted')
  New-Item -ItemType Directory -Force -Path $InstallDirectory | Out-Null
  Copy-Item (Join-Path $workDirectory 'extracted\supabase-toys.exe') (Join-Path $InstallDirectory 'supabase-toys.exe') -Force
  Write-Host "Installed $Version to $InstallDirectory. Add this directory to PATH and run supabase-toys --help."
} finally { Remove-Item $workDirectory -Recurse -Force }

