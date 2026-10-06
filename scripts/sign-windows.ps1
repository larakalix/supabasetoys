$ErrorActionPreference = 'Stop'
if (!$env:WINDOWS_CERTIFICATE -or !$env:WINDOWS_CERTIFICATE_PASSWORD) { throw 'Windows signing secrets are required for release' }
$path = Join-Path $env:RUNNER_TEMP 'toys-signing.pfx'
try {
 [IO.File]::WriteAllBytes($path, [Convert]::FromBase64String($env:WINDOWS_CERTIFICATE))
 $password = ConvertTo-SecureString $env:WINDOWS_CERTIFICATE_PASSWORD -AsPlainText -Force
 $cert = Import-PfxCertificate -FilePath $path -CertStoreLocation Cert:\CurrentUser\My -Password $password
 $signTool = Get-ChildItem 'C:\Program Files (x86)\Windows Kits\10\bin\*\x64\signtool.exe' | Sort-Object FullName -Descending | Select-Object -First 1
 if (!$signTool) { throw 'Windows SDK signtool is required' }
 $binary = Get-Item 'build/bin/supabase-toys-desktop.exe'
 function Sign-Artifact($file) {
  & $signTool.FullName sign /sha1 $cert.Thumbprint /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 $file.FullName
  if ($LASTEXITCODE -ne 0) { throw 'Windows code signing failed' }
  & $signTool.FullName verify /pa $file.FullName
  if ($LASTEXITCODE -ne 0) { throw 'Windows signature verification failed' }
 }
 Sign-Artifact $binary
 # Repackage after signing so the installed application also carries its signature.
 Push-Location build/windows/installer
 try {
  & makensis ("-DARG_WAILS_AMD64_BINARY=" + $binary.FullName) 'project.nsi'
  if ($LASTEXITCODE -ne 0) { throw 'Signed NSIS repackaging failed' }
 } finally { Pop-Location }
 $installers = @(Get-ChildItem build/bin -Filter '*installer.exe')
 if (!$installers.Count) { throw 'Expected an NSIS installer' }
 foreach ($installer in $installers) { Sign-Artifact $installer }
 New-Item -ItemType Directory -Force release-assets | Out-Null
 Get-ChildItem build/bin -Filter '*installer.exe' | Copy-Item -Destination release-assets
 if (!(Get-ChildItem release-assets -Filter '*.exe')) { throw 'Signed installer asset not found' }
} finally {
 Remove-Item $path -Force -ErrorAction SilentlyContinue
}
