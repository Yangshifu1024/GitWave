package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareSetupKeepsTheLegacyArgumentsAndRoutesTheInstallThroughGo(t *testing.T) {
	const original = `OutFile "old.exe"
InstallDirRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\desktop.gitwave.work" "InstallLocation"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_FINISH
VIAddVersionKey /LANG=${LANG_ENGLISH} "ProductVersion" "0.12.0"
Function .onInit
FunctionEnd
Section
  Call CloseApp
  WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd
Section "Uninstall"
  Call un.CloseApp
  ExecWait '"$INSTDIR\GitWave.exe" --nsgo-unregister-toast'
  Delete "$INSTDIR\THIRD_PARTY_NOTICES.txt"
SectionEnd
`
	prepared, name, err := prepareSetup(original, t.TempDir())
	if err != nil || name != "gitwave-0.12.0-windows-x64-setup.exe" {
		t.Fatalf("prepared setup %q: %v", name, err)
	}
	for _, value := range []string{
		`\Uninstall\GitWave" "InstallLocation"`, `"/P"`, `"/R"`, `"/ARGS"`,
		`--gitwave-install "$INSTDIR" "desktop.gitwave.work" "0.12.0"`, `--gitwave-prepare-uninstall`,
		`WriteUninstaller "$OUTDIR\Uninstall.exe"`, `SetOutPath "$PLUGINSDIR\GitWave\gitwave-git"`,
		`SetOutPath "$PLUGINSDIR\GitWaveUninstall"`, `--gitwave-prepare-uninstall "$INSTDIR"`,
		`ExecWait '"$R0" --nsgo-unregister-toast'`,
	} {
		if !strings.Contains(prepared, value) {
			t.Errorf("prepared setup is missing %q", value)
		}
	}
	if strings.Count(prepared, "!insertmacro GitWaveLockSetup") != 2 {
		t.Fatal("the install and uninstall sections must both hold the setup mutex")
	}
	if strings.Contains(prepared, `Delete "$INSTDIR\THIRD_PARTY_NOTICES.txt"`) {
		t.Fatal("uninstall would remove a same-named user notice at the installation root")
	}
	preview := strings.ReplaceAll(original, "desktop.gitwave.work", "desktop.gitwave.work.native-preview")
	prepared, _, err = prepareSetup(preview, t.TempDir())
	if err != nil || strings.Contains(prepared, `\Uninstall\GitWave" "InstallLocation"`) {
		t.Fatalf("preview consulted the production installation: %v", err)
	}
	if _, _, err := prepareSetup(strings.Replace(original, "Call CloseApp", "Quit", 1), t.TempDir()); err == nil {
		t.Fatal("unexpected upstream setup structure was accepted")
	}
}

func TestReleaseSignHookRequiresTheStableRequestedVersion(t *testing.T) {
	const source = `InstallDirRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\desktop.gitwave.work" "InstallLocation"
VIAddVersionKey /LANG=${LANG_ENGLISH} "ProductVersion" "0.12.0"
`
	signer := filepath.Join(t.TempDir(), "fixture signer.exe")
	if hook, err := releaseSignHook("\ufeff"+strings.ReplaceAll(source, "\n", "\r\n"), "0.12.0", signer); err != nil ||
		hook != "!uninstfinalize '\""+signer+"\" \"%1\"' = 0\n" {
		t.Fatalf("signed uninstaller hook = %q, %v", hook, err)
	}
	for _, test := range []struct{ script, version, signer string }{
		{strings.ReplaceAll(source, "desktop.gitwave.work", "desktop.gitwave.work.native-preview"), "0.12.0", signer},
		{source, "0.12.1", signer},
		{source, "0.12.0-rc.1", signer},
		{source + "!uninstfinalize 'other' = 0\n", "0.12.0", signer},
		{source, "0.12.0", signer + "\n!echo unsafe"},
	} {
		if _, err := releaseSignHook(test.script, test.version, test.signer); err == nil {
			t.Fatalf("accepted unexpected signing input: %+v", test)
		}
	}
}
