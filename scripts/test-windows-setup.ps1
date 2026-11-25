param(
  [Parameter(Mandatory)][string]$Setup,
  [Parameter(Mandatory)][string]$Version,
  [Parameter(Mandatory)][string]$Previous,
  [Parameter(Mandatory)][string]$Fixture,
  [string]$SignedApp
)

$ErrorActionPreference = 'Stop'

function Test-SignedBinary([string]$Path) {
  $signature = Get-AuthenticodeSignature -LiteralPath $Path
  if ($signature.Status -ne 'Valid' -or !$signature.SignerCertificate -or !$signature.TimeStamperCertificate) {
    throw "Invalid Authenticode signature or timestamp for $Path ($($signature.Status))"
  }
  return $signature.SignerCertificate.Subject
}

function Get-CheckedHash([string]$Path) {
  $actual = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash
  $line = (Get-Content -LiteralPath "$Path.sha256" -Raw).TrimEnd()
  $expected = "$actual  $(Split-Path -Leaf $Path)"
  if ($line -ne $expected) { throw "SHA-256 mismatch for $Path" }
  return $actual
}

if ((Get-FileHash -LiteralPath $Previous).Hash -ne '4f70084024d8d987be4a7c1603eddbdce12e07245420fbbeaafa5a462d739966') {
  throw 'Legacy installer SHA-256 mismatch'
}
if ((Get-Item -LiteralPath $Setup).VersionInfo.ProductVersion -ne $Version) {
  throw "The NSIS installer does not report ProductVersion $Version"
}
if ($SignedApp) {
  Get-CheckedHash $Setup | Out-Null
  $appHash = Get-CheckedHash $SignedApp
  $publisher = Test-SignedBinary $SignedApp
  if ((Test-SignedBinary $Setup) -cne $publisher) { throw 'The installer uses a different publisher' }
  if ((Get-Item -LiteralPath $SignedApp).VersionInfo.ProductVersion -ne $Version) {
    throw "The signed GitWave.exe does not report ProductVersion $Version"
  }
}

$install = Join-Path $Fixture 'Custom GitWave installation'
$root = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall'
$legacy = Join-Path $root 'GitWave'
$stable = Join-Path $root 'desktop.gitwave.work'
if ((Test-Path -LiteralPath $install) -or (Test-Path $legacy) -or (Test-Path $stable)) {
  throw 'Start the setup test in a clean runner profile and fixture directory'
}
$previousSetup = Start-Process $Previous -ArgumentList "/S /D=$install" -Wait -PassThru
if ($previousSetup.ExitCode -ne 0) { throw 'v0.11.0 setup failed' }
if ((Get-ItemPropertyValue $legacy InstallLocation).Trim('"') -ne $install) {
  throw 'v0.11.0 did not register the custom installation path'
}
$userFile = Join-Path $install 'user-file.txt'
Set-Content -LiteralPath $userFile 'user-owned file'
$userNotices = Join-Path $install 'THIRD_PARTY_NOTICES.txt'
Set-Content -LiteralPath $userNotices 'user-owned notice'
$upgrade = Start-Process $Setup -ArgumentList '/S /UPDATE' -Wait -PassThru
if ($upgrade.ExitCode -ne 0 -or (Test-Path $legacy)) { throw 'The Go setup did not migrate the old registration' }
if ((Get-ItemPropertyValue $stable InstallLocation) -ne $install -or
    (Get-ItemPropertyValue $stable DisplayVersion) -ne $Version) {
  throw 'The Go setup did not upgrade in place'
}
$application = Join-Path $install 'GitWave.exe'
if ((Get-Item -LiteralPath $application).VersionInfo.ProductVersion -ne $Version -or
    (Get-Content -LiteralPath $userFile) -ne 'user-owned file' -or
    (Get-Content -LiteralPath $userNotices) -ne 'user-owned notice') {
  throw 'The upgrade did not publish the new app or preserve user files'
}
if (!(Test-Path -LiteralPath (Join-Path $install 'gitwave-git/cmd/git.exe'))) {
  throw 'The upgrade did not install the Git bundle'
}
$notices = Join-Path $install 'gitwave-git/GitWave-THIRD_PARTY_NOTICES.txt'
if (!(Test-Path -LiteralPath $notices -PathType Leaf) -or
    !(Select-String -LiteralPath $notices -Pattern 'modernc.org/sqlite' -SimpleMatch -Quiet)) {
  throw 'The upgrade did not install the dependency license notices'
}
$installedUninstaller = Join-Path $install 'Uninstall.exe'
if ($SignedApp) {
  if ((Get-FileHash -LiteralPath $application -Algorithm SHA256).Hash -ne $appHash) {
    throw 'The installed EXE differs from the signed manifest input'
  }
  if ((Test-SignedBinary $installedUninstaller) -cne $publisher) {
    throw 'The embedded uninstaller uses a different publisher'
  }
}
$uninstaller = Join-Path $Fixture 'GitWave uninstall fixture.exe'
Copy-Item -LiteralPath $installedUninstaller -Destination $uninstaller
$uninstall = Start-Process $uninstaller -ArgumentList "/S _?=$install" -Wait -PassThru
if ($uninstall.ExitCode -ne 0 -or (Test-Path $stable) -or (Test-Path -LiteralPath $application) -or
    (Test-Path -LiteralPath $notices) -or (Get-Content -LiteralPath $userFile) -ne 'user-owned file' -or
    (Get-Content -LiteralPath $userNotices) -ne 'user-owned notice') {
  throw 'The migrated installation failed to uninstall or removed a user-owned file'
}
