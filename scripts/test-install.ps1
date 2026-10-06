# Offline PowerShell installer smoke test. Run after building the Windows CLI.
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$work = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid().ToString())
New-Item -ItemType Directory $work | Out-Null
try {
  $script:fixtureAssets = Join-Path $work 'assets'
  $payload = Join-Path $work 'payload'
  $destination = Join-Path $work 'installed'
  New-Item -ItemType Directory $fixtureAssets, $payload | Out-Null
  Copy-Item (Join-Path $root 'build\bin\supabase-toys') (Join-Path $payload 'supabase-toys.exe')
  $assetName = 'supabase-toys_v0.1.0_windows_x64.zip'
  $archive = Join-Path $fixtureAssets $assetName
  Compress-Archive (Join-Path $payload 'supabase-toys.exe') $archive
  $checksumPath = Join-Path $fixtureAssets 'SHA256SUMS'
  "$( (Get-FileHash $archive -Algorithm SHA256).Hash.ToLower() )  $assetName" | Set-Content $checksumPath
  function Invoke-WebRequest {
    param([string]$Uri, [string]$OutFile)
    Copy-Item (Join-Path $script:fixtureAssets ($Uri.Split('/')[-1])) $OutFile
  }
  $installer = Join-Path $PSScriptRoot 'install.ps1'
  & $installer -Repository 'example/toys' -Version 'v0.1.0' -InstallDirectory $destination
  $binary = Join-Path $destination 'supabase-toys.exe'
  & $binary --help | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'Installed CLI could not launch' }
  $before = (Get-FileHash $binary -Algorithm SHA256).Hash
  (('0' * 64) + "  $assetName") | Set-Content $checksumPath
  $rejected = $false
  try { & $installer -Repository 'example/toys' -Version 'v0.1.0' -InstallDirectory $destination }
  catch {
    if ($_.Exception.Message -notmatch 'Checksum verification failed') { throw }
    $rejected = $true
  }
  if (!$rejected) { throw 'Bad checksum was accepted' }
  if ((Get-FileHash $binary -Algorithm SHA256).Hash -ne $before) { throw 'Failed verification changed the installed binary' }
  Write-Host 'PASS: PowerShell installation, CLI launch, and checksum rejection'
} finally {
  Remove-Item Function:Invoke-WebRequest -ErrorAction SilentlyContinue
  Remove-Item $work -Recurse -Force
}
