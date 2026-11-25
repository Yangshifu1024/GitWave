package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"gitwave.work/internal/appupdate"
	"gitwave.work/internal/desktopgtk"
	"gitwave.work/internal/distasset"
)

// The pinned NSGo CLI flattens desktop resource symlinks. Patch a private copy
// used to build the installer tool, keeping the app's SDK and module unchanged.
func buildNSGoPackager(work string) (string, error) {
	const nsgoModule = "github.com/nsgo-dev/nsgo"
	const cli = nsgoModule + "/cmd/nsgo"
	output, err := exec.Command("go", "list", "-f", "{{.Dir}}", cli).Output()
	if err != nil {
		return "", fmt.Errorf("locate the NSGo packager: %w", err)
	}
	cliDir := strings.TrimSpace(string(output))
	if !filepath.IsAbs(cliDir) {
		return "", fmt.Errorf("invalid NSGo package directory %q", cliDir)
	}
	moduleDir := filepath.Dir(filepath.Dir(cliDir))
	private := filepath.Join(work, "nsgo-source")
	if err := os.CopyFS(private, os.DirFS(moduleDir)); err != nil {
		return "", err
	}
	resources := filepath.Join(private, "cmd", "nsgo", "resources.go")
	source, err := os.ReadFile(resources)
	if err != nil {
		return "", err
	}
	updated, err := patchResources(string(source))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(resources, []byte(updated), 0o600); err != nil {
		return "", err
	}
	if runtime.GOOS == "linux" {
		if err := patchLinuxPreviewSlug(filepath.Join(private, "cmd", "nsgo", "installer.go")); err != nil {
			return "", err
		}
		if err := patchDebPackage(filepath.Join(private, "cmd", "nsgo", "linuxpkg.go")); err != nil {
			return "", err
		}
	}
	modfile := filepath.Join(work, "packager.mod")
	for _, ext := range []string{".mod", ".sum"} {
		data, err := os.ReadFile("go" + ext)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(work, "packager"+ext), data, 0o600); err != nil {
			return "", err
		}
	}
	if err := run(exec.Command("go", "mod", "edit", "-modfile", modfile, "-replace", nsgoModule+"="+private)); err != nil {
		return "", err
	}
	packager := filepath.Join(work, "nsgo")
	return packager, run(exec.Command("go", "build", "-modfile", modfile, "-o", packager, cli))
}

func patchDebPackage(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	const original = `linuxDependency("libgtk-4-1", p.Linux.Depends.Deb)`
	const packageField = `p.packageSlug(), debVersion(p.Version), debArch[arch],`
	const desktopEntry = `add("usr/share/applications/"+p.ID+".desktop", desktop.Bytes(), 0o644)`
	if strings.Count(string(data), original) != 1 || strings.Count(string(data), packageField) != 1 || strings.Count(string(data), desktopEntry) != 1 {
		return fmt.Errorf("the NSGo Debian control fields changed, check package compatibility")
	}
	const replacement = `linuxDependency("libgtk-4-1", gitWaveDebGTKDeps(p.Linux.Depends.Deb))`
	const helper = `
func gitWaveDebGTKDeps(deps []string) []string {
	if os.Getenv("` + gtkEnvironment + `") == "" {
		return deps
	}
	const modern = "libgtk-4-1 (>= 4.14)"
	deps = slices.DeleteFunc(slices.Clone(deps), func(dep string) bool { return dep == modern })
	return append(deps, modern+" | libtiff5", modern+" | libjpeg-turbo8", modern+" | libcairo-script-interpreter2")
}

func gitWaveDebPackageName(p *project) string {
	if p.ID == "` + appupdate.DesktopAppID + `" {
		return "` + distasset.LinuxPackageName + `"
	}
	return p.packageSlug()
}
`
	updated := strings.Replace(string(data), original, replacement, 1)
	updated = strings.Replace(updated, packageField, `gitWaveDebPackageName(p), debVersion(p.Version), debArch[arch],`, 1)
	updated = strings.Replace(updated, desktopEntry, desktopEntry+`
	if p.ID == "`+appupdate.DesktopAppID+`" {
		// Preserve launchers recorded using the original desktop file ID.
		alias := bytes.Replace(desktop.Bytes(), []byte("[Desktop Entry]\n"), []byte("[Desktop Entry]\nNoDisplay=true\n"), 1)
		add("usr/share/applications/GitWave.desktop", alias, 0o644)
	}`, 1)
	return os.WriteFile(path, []byte(updated+helper), 0o600)
}

func patchLinuxPreviewSlug(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	const start = "func (p *project) packageSlug() string {\n"
	if strings.Count(string(data), start) != 1 {
		return fmt.Errorf("the NSGo package slug changed, check Linux preview isolation")
	}
	const usePreviewSlug = "\tif p.ID == \"desktop.gitwave.work.native-preview\" {\n\t\treturn \"gitwave-preview\"\n\t}\n"
	updated := strings.Replace(string(data), start, start+usePreviewSlug, 1)
	return os.WriteFile(path, []byte(updated), 0o600)
}

func patchResources(source string) (string, error) {
	const imports = "import (\n"
	const hook = "func copyResources(p *project, root string) error {"
	if strings.Count(source, imports) != 1 || strings.Count(source, hook) != 1 {
		return "", fmt.Errorf("the NSGo desktop resource hook changed, check the installer packager")
	}
	source = strings.Replace(source, imports, imports+"\t\"context\"\n\t\"debug/macho\"\n\t\"runtime\"\n", 1)
	source = strings.Replace(source, hook, "func copyNSGoResources(p *project, root string) error {", 1)
	return source + gitWaveResourceHook, nil
}

const gitWaveResourceHook = `
func copyResources(p *project, root string) error {
	if err := copyNSGoResources(p, root); err != nil {
		return err
	}
	if gtk := os.Getenv("` + gtkEnvironment + `"); gtk != "" {
		if err := os.CopyFS(filepath.Join(root, "` + desktopgtk.Directory + `"), os.DirFS(gtk)); err != nil {
			return err
		}
	}
	return copyGitWaveTools(p, root)
}

func copyGitWaveTools(p *project, root string) error {
	source, err := filepath.EvalSymlinks(os.Getenv("` + bundleEnvironment + `"))
	if err != nil || !filepath.IsAbs(source) {
		return fmt.Errorf("locate the staged Git toolchain: %v", err)
	}
	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.Type()&fs.ModeSymlink == 0 {
			return err
		}
		link, err := os.Readlink(path)
		if err != nil {
			return err
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, resolved)
		if filepath.IsAbs(link) || err != nil || !filepath.IsLocal(rel) {
			return fmt.Errorf("Git bundle symlink leaves the package: %s", path)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	destination := filepath.Join(root, "gitwave-git")
	if err := os.Mkdir(destination, 0o755); err != nil {
		return err
	}
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil || runtime.GOOS != "darwin" {
		return err
	}
	return filepath.WalkDir(destination, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		info, err := entry.Info()
		if err != nil || info.Mode().Perm()&0o111 == 0 {
			return err
		}
		bin, err := macho.Open(path)
		if err != nil {
			return nil // Git also installs executable shell and Perl scripts.
		}
		minimum := gitWaveMachOMinimum(bin)
		if err := bin.Close(); err != nil {
			return err
		}
		if minimum == 0 || minimum > 13 << 16 {
			return fmt.Errorf("%s does not declare support for macOS ` + macOSMinimum + `", path)
		}
		args := p.macSignArgs(path)
		if p.Sign.Identity == "" || p.Sign.Identity == "-" {
			args = []string{"--force", "--sign", "-", "--options", "runtime", path}
		}
		return run(context.Background(), "codesign", args...)
	})
}

func gitWaveMachOMinimum(file *macho.File) uint32 {
	for _, load := range file.Loads {
		data := load.Raw()
		switch file.ByteOrder.Uint32(data[:4]) {
		case 0x32: // LC_BUILD_VERSION
			if len(data) >= 16 && file.ByteOrder.Uint32(data[8:12]) == 1 {
				return file.ByteOrder.Uint32(data[12:16])
			}
		case 0x24: // LC_VERSION_MIN_MACOSX
			if len(data) >= 12 {
				return file.ByteOrder.Uint32(data[8:12])
			}
		}
	}
	return 0
}
`
