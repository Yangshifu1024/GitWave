package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gitwave.work/internal/appupdate"
	"gitwave.work/internal/distasset"
	"gitwave.work/internal/gittool"
)

const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\`
const thirdPartyNoticesName = "THIRD_PARTY_NOTICES.txt"

var (
	setupIdentity = regexp.MustCompile(`(?m)^InstallDirRegKey HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\(desktop\.gitwave\.work(?:\.native-preview)?)" "InstallLocation"$`)
	setupVersion  = regexp.MustCompile(`(?m)^VIAddVersionKey /LANG=\$\{LANG_ENGLISH\} "ProductVersion" "([^"\r\n]+)"$`)
	setupOutput   = regexp.MustCompile(`(?m)^OutFile "[^"\r\n]+"$`)
)

func main() {
	input := flag.String("script", "", "NSGo-generated GitWave.nsi")
	output := flag.String("out", "", "directory for the compiled setup and SHA-256 file")
	compiler := flag.String("makensis", "makensis", "path to the NSIS compiler")
	gitAssets := flag.String("git-assets", "", "directory with the pinned Git downloads for an offline build")
	version := flag.String("release-version", "", "build a signed Windows release with this version")
	signer := flag.String("signer", "", "executable that Authenticode-signs the file supplied as its argument")
	flag.Parse()
	if err := compileSetup(*input, *output, *compiler, *gitAssets, *version, *signer); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func compileSetup(input, output, compiler, gitAssets, release, signer string) error {
	if input == "" || output == "" {
		return errors.New("-script and -out are required")
	}
	source, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	if (release == "") != (signer == "") {
		return errors.New("-release-version and -signer must be provided together")
	}
	var signHook string
	if release != "" {
		signer, err = exec.LookPath(signer)
		if err != nil {
			return err
		}
		signer, err = filepath.Abs(signer)
		if err != nil {
			return err
		}
		signHook, err = releaseSignHook(string(source), release, signer)
		if err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(filepath.Dir(input))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "GitWave.exe", "GitWave.nsi", "GitWave-setup.ico", "GitWave-uninstall.ico", thirdPartyNoticesName:
			continue
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "-setup.exe") {
			continue
		}
		return fmt.Errorf("the NSGo package contains an installation file without a rollback rule: %s", entry.Name())
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	publication := output
	if release != "" {
		if _, err := os.Lstat(output); !os.IsNotExist(err) {
			if err != nil {
				return err
			}
			return fmt.Errorf("the Windows release output already exists: %s", output)
		}
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return err
		}
		publication, err = os.MkdirTemp(filepath.Dir(output), ".gitwave-windows-release-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(publication)
	}
	if err := os.MkdirAll(publication, 0o755); err != nil {
		return err
	}
	work, err := os.MkdirTemp(publication, ".gitwave-nsis-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	compilerDir := filepath.Dir(input)
	if release != "" {
		compilerDir, err = stageSignedWindowsApp(compilerDir, work, signer)
		if err != nil {
			return err
		}
	}
	prepared, artifact, err := prepareSetup(string(source), work)
	if err != nil {
		return err
	}
	if err := stageWindowsGit(work, gitAssets, filepath.Join(compilerDir, thirdPartyNoticesName)); err != nil {
		return fmt.Errorf("stage Windows Git: %w", err)
	}
	path := filepath.Join(work, "GitWave.nsi")
	if err := os.WriteFile(path, []byte(signHook+prepared), 0o600); err != nil {
		return err
	}
	option := "-"
	if filepath.Separator == '\\' {
		option = "/"
	}
	cmd := exec.Command(compiler, option+"NOCD", option+"INPUTCHARSET", "UTF8", path)
	cmd.Dir = compilerDir
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("compile GitWave NSIS setup: %w", err)
	}
	setup := filepath.Join(work, artifact)
	if release != "" {
		if err := signWindowsFile(signer, setup); err != nil {
			return err
		}
		if err := collectWindowsArtifact(filepath.Join(compilerDir, "GitWave.exe"), publication, "GitWave.exe"); err != nil {
			return err
		}
	}
	if err := collectWindowsArtifact(setup, publication, artifact); err != nil {
		return err
	}
	if release == "" {
		return nil
	}
	if err := os.RemoveAll(work); err != nil {
		return err
	}
	return distasset.PublishDirectory(publication, output)
}

func collectWindowsArtifact(path, output, name string) error {
	pkg, err := appupdate.DescribePackage(path)
	if err != nil {
		return err
	}
	if err := os.Rename(path, filepath.Join(output, name)); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(output, name+".sha256"), []byte(pkg.SHA256+"  "+name+"\n"), 0o644)
}

func prepareSetup(script, work string) (string, string, error) {
	script = strings.ReplaceAll(strings.TrimPrefix(script, "\ufeff"), "\r\n", "\n")
	id, version := setupIdentity.FindStringSubmatch(script), setupVersion.FindStringSubmatch(script)
	if len(id) != 2 || len(version) != 2 || len(setupOutput.FindAllString(script, 2)) != 1 {
		return "", "", errors.New("the NSGo installer has an unexpected registration or version format")
	}
	artifact, err := appupdate.ArtifactName(version[1], appupdate.WindowsX64)
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(work, artifact)
	if strings.ContainsAny(path, "\r\n\"") {
		return "", "", errors.New("the installer path contains an invalid character")
	}
	head, install, ok := strings.Cut(script, "\nSection\n  Call CloseApp\n")
	_, tail, hasEnd := strings.Cut(install, "\nSectionEnd\n")
	if !ok || !hasEnd || !strings.Contains(install, `WriteUninstaller "$INSTDIR\Uninstall.exe"`) ||
		!strings.Contains(head, "Function .onInit\nFunctionEnd") || !strings.Contains(tail, "Section \"Uninstall\"\n  Call un.CloseApp\n") {
		return "", "", errors.New("the NSGo installer structure changed")
	}
	// Reuse NSGo's language resources, branding and uninstaller. Only the
	// install body and its page behavior differ for GitWave's update flow.
	head = setupOutput.ReplaceAllStringFunc(head, func(string) string { return `OutFile "` + strings.ReplaceAll(path, "$", "$$") + `"` })
	head = strings.Replace(head, "!insertmacro MUI_PAGE_DIRECTORY", "!define MUI_PAGE_CUSTOMFUNCTION_PRE GitWaveSkipPassive\n!insertmacro MUI_PAGE_DIRECTORY", 1)
	head = strings.Replace(head, "!insertmacro MUI_PAGE_FINISH", "!define MUI_PAGE_CUSTOMFUNCTION_PRE GitWaveSkipPassive\n!insertmacro MUI_PAGE_FINISH", 1)
	head = strings.Replace(head, "Function .onInit\nFunctionEnd", initSetup(id[1] == "desktop.gitwave.work"), 1)
	if before, body, found := strings.Cut(head, "!macro CloseApp un\n"); found {
		_, after, end := strings.Cut(body, "!insertmacro CloseApp \"un.\"\n")
		if !end {
			return "", "", errors.New("the NSGo process-closing macro changed")
		}
		head = before + after
	}
	head = strings.Replace(head, "!insertmacro RemoveFileTypes \"\"\n", "", 1)
	tail = strings.Replace(tail, "  Call un.CloseApp\n", `  !insertmacro GitWaveLockSetup
  StrCpy $R0 "$INSTDIR\GitWave.exe"
  IfFileExists "$R0" gitwave_run_uninstall_worker
  InitPluginsDir
  SetOutPath "$PLUGINSDIR\GitWaveUninstall"
  File "GitWave.exe"
  StrCpy $R0 "$OUTDIR\GitWave.exe"
  gitwave_run_uninstall_worker:
  ClearErrors
  ExecWait '"$R0" --gitwave-prepare-uninstall "$INSTDIR"' $0
  ${If} ${Errors}
  ${OrIf} $0 != 0
    DetailPrint "Close GitWave and check the bundled Git directory before uninstalling."
    SetErrorLevel 2
    Abort
  ${EndIf}
`, 1)
	tail = strings.Replace(tail, `ExecWait '"$INSTDIR\GitWave.exe" --nsgo-unregister-toast'`,
		`ExecWait '"$R0" --nsgo-unregister-toast'`, 1)
	// Setup and uninstall manage the notices inside the bundled Git directory.
	// Keep any same-named user file at the installation root.
	tail = strings.Replace(tail, "  Delete \"$INSTDIR\\"+thirdPartyNoticesName+"\"\n", "", 1)
	bundle := strings.ReplaceAll(filepath.Join(work, gittool.Directory), "$", "$$")
	return head + setupMutexMacro + "\n" + installSection(id[1], version[1], bundle) + tail, artifact, nil
}

// Keep the mutex until NSIS exits, including its actions after the Go worker.
const setupMutexMacro = `
!macro GitWaveLockSetup
  System::Call 'kernel32::CreateMutexW(p 0, i 0, w "Local\GitWave.PackageSetup.1") p.r8'
  ${If} $8 != 0
    System::Call 'kernel32::WaitForSingleObject(p r8, i 0) i.r9'
    ${If} $9 == 0
    ${OrIf} $9 == 128
      Goto gitwave_setup_locked
    ${EndIf}
    System::Call 'kernel32::CloseHandle(p r8)'
  ${EndIf}
  DetailPrint "Another GitWave setup or uninstall is already running."
  SetErrorLevel 2
  Abort
  gitwave_setup_locked:
!macroend
`

func initSetup(legacy bool) string {
	legacyPath := ""
	if legacy {
		legacyPath = `
  System::Call 'kernel32::GetCommandLineW() t.r2'
  ClearErrors
  ${GetOptionsS} $2 "/D=" $3
  ${If} ${Errors}
    ReadRegStr $1 HKCU "` + uninstallKey + `desktop.gitwave.work" "InstallLocation"
    ${If} $1 == ""
      ReadRegStr $1 HKCU "` + uninstallKey + `GitWave" "InstallLocation"
      StrCpy $2 $1 1
      StrCmp $2 '$\"' 0 +2
      StrCpy $1 $1 -1 1
      IfFileExists "$1\GitWave.exe" 0 +2
      StrCpy $INSTDIR $1
    ${EndIf}
  ${EndIf}`
	}
	return `Var GitWavePassive
Var GitWaveRestart
Var GitWaveArgs

Function GitWaveSkipPassive
  ${If} $GitWavePassive == 1
    Abort
  ${EndIf}
FunctionEnd

Function .onInit
  ${GetParameters} $0
  ClearErrors
  ${GetOptionsS} "$0" "/P" $1
  ${IfNot} ${Errors}
    StrCpy $GitWavePassive 1
    SetAutoClose true
  ${EndIf}
  ClearErrors
  ${GetOptionsS} "$0" "/R" $1
  ${IfNot} ${Errors}
    StrCpy $GitWaveRestart 1
    ${GetOptions} "$0" "/ARGS" $GitWaveArgs
  ${Else}
    ClearErrors
    ${GetOptionsS} "$0" "/RELAUNCH" $1
    ${IfNot} ${Errors}
      StrCpy $GitWaveRestart 1
    ${EndIf}
  ${EndIf}` + legacyPath + `
FunctionEnd`
}

func installSection(id, version, bundle string) string {
	return `Section
  !insertmacro GitWaveLockSetup
  InitPluginsDir
  SetOutPath "$PLUGINSDIR\GitWave"
  File "GitWave.exe"
  ClearErrors
  SetOutPath "$PLUGINSDIR\GitWave\` + gittool.Directory + `"
  File /r "` + filepath.Join(bundle, "*") + `"
  SetOutPath "$PLUGINSDIR\GitWave"
  WriteUninstaller "$OUTDIR\Uninstall.exe"
  ${If} ${Errors}
    SetErrorLevel 2
    Abort
  ${EndIf}
  ClearErrors
  ExecWait '"$OUTDIR\GitWave.exe" --gitwave-install "$INSTDIR" "` + id + `" "` + version + `"' $0
  ${If} ${Errors}
  ${OrIf} $0 != 0
    ClearErrors
    FileOpen $1 "$OUTDIR\install-error.txt" r
    ${IfNot} ${Errors}
      FileRead $1 $2
      FileClose $1
      DetailPrint "$2"
    ${EndIf}
    SetErrorLevel 2
    Abort
  ${EndIf}
  SetOutPath "$INSTDIR"
  CreateShortCut "$SMPROGRAMS\GitWave.lnk" "$INSTDIR\GitWave.exe"
  ExecWait '"$INSTDIR\GitWave.exe" --nsgo-register-toast="$SMPROGRAMS\GitWave.lnk"'
  ${If} $GitWaveRestart == 1
    Exec '"$INSTDIR\GitWave.exe" $GitWaveArgs'
  ${EndIf}
SectionEnd
`
}
