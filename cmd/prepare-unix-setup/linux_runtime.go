package main

import (
	"debug/elf"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Keep the Git loader's private library path on its ELF files. SSH and
// credential helpers started from Git still use their normal system libraries.
func stageLinuxRuntime(bundle string) error {
	var programs []string
	libraries := make(map[string]string)
	err := filepath.WalkDir(bundle, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		binary, err := elf.Open(path)
		if err != nil {
			return nil
		}
		defer binary.Close()
		if err := checkLinuxLibc(binary, path); err != nil {
			return err
		}
		needed, err := binary.ImportedLibraries()
		if err != nil || len(needed) == 0 {
			return err
		}
		programs = append(programs, path)
		return collectLinuxLibraries(path, libraries)
	})
	if err != nil {
		return err
	}
	if libraries["libcurl.so.4"] == "" {
		return fmt.Errorf("the Linux Git build is missing its HTTPS runtime")
	}
	libDir := filepath.Join(bundle, "lib")
	if err := os.Mkdir(libDir, 0o755); err != nil {
		return err
	}
	var names []string
	for name := range libraries {
		names = append(names, name)
	}
	sort.Strings(names)
	var provenance strings.Builder
	provenance.WriteString("Libraries copied from the Ubuntu 22.04 build environment\n\nLibrary\tSource package\tSource version\tBinary package\n")
	for _, name := range names {
		original := libraries[name]
		target := filepath.Join(libDir, name)
		data, err := os.ReadFile(original)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		if err := run(exec.Command("patchelf", "--set-rpath", "$ORIGIN", target)); err != nil {
			return err
		}
		if err := recordLinuxLibrary(bundle, original, name, &provenance); err != nil {
			return err
		}
	}
	for _, program := range programs {
		relative, err := filepath.Rel(filepath.Dir(program), libDir)
		if err != nil {
			return err
		}
		if err := run(exec.Command("patchelf", "--set-rpath", "$ORIGIN/"+relative, program)); err != nil {
			return err
		}
	}
	resolved := make(map[string]string)
	for _, program := range programs {
		if err := collectLinuxLibraries(program, resolved); err != nil {
			return err
		}
	}
	for _, name := range names {
		if expected := filepath.Join(libDir, name); resolved[name] != expected {
			return fmt.Errorf("%s resolved to %q instead of the packaged library", name, resolved[name])
		}
	}
	return os.WriteFile(filepath.Join(bundle, "GitWave-third-party-libraries.txt"), []byte(provenance.String()), 0o644)
}

func checkLinuxLibc(binary *elf.File, path string) error {
	symbols, err := binary.ImportedSymbols()
	if err != nil {
		return err
	}
	for _, symbol := range symbols {
		version, ok := strings.CutPrefix(symbol.Version, "GLIBC_")
		if !ok {
			continue
		}
		major, minor, _ := strings.Cut(version, ".")
		minor, _, _ = strings.Cut(minor, ".")
		number, err := strconv.Atoi(minor)
		if err != nil || major != "2" || number > 35 {
			return fmt.Errorf("%s requires %s, build the Linux Git bundle on Ubuntu 22.04", path, symbol.Version)
		}
	}
	return nil
}

func collectLinuxLibraries(program string, libraries map[string]string) error {
	cmd := exec.Command("ldd", program)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "LD_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("inspect the Linux dependencies of %s: %w", program, err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		name, detail, found := strings.Cut(strings.TrimSpace(line), "=>")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		path, _, _ := strings.Cut(strings.TrimSpace(detail), " (")
		if !filepath.IsAbs(path) || filepath.Base(name) != name {
			return fmt.Errorf("unresolved Linux dependency of %s: %s", program, strings.TrimSpace(line))
		}
		switch name {
		case "libc.so.6", "libm.so.6", "libpthread.so.0", "libdl.so.2", "librt.so.1", "libresolv.so.2", "libutil.so.1":
			continue // glibc and the ELF loader come from the same host.
		}
		path = filepath.Clean(path)
		if previous, found := libraries[name]; found && previous != path {
			return fmt.Errorf("conflicting Linux dependencies for %s: %s and %s", name, previous, path)
		}
		libraries[name] = path
	}
	return nil
}

func recordLinuxLibrary(bundle, path, name string, provenance *strings.Builder) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	var owner string
	for _, candidate := range []string{path, resolved, strings.TrimPrefix(resolved, "/usr")} {
		output, err := exec.Command("dpkg-query", "-S", candidate).Output()
		if err == nil {
			owner, _, _ = strings.Cut(strings.TrimSpace(string(output)), ": ")
			break
		}
	}
	if owner == "" {
		return fmt.Errorf("no Ubuntu package claims %s", path)
	}
	output, err := exec.Command("dpkg-query", "-W", "-f=${source:Package}\t${source:Version}\t${binary:Package}\n", owner).Output()
	if err != nil {
		return fmt.Errorf("find the source package for %s: %w", name, err)
	}
	fmt.Fprintf(provenance, "%s\t%s", name, output)
	pkg, _, _ := strings.Cut(owner, ":")
	copyright, err := os.ReadFile(filepath.Join("/usr/share/doc", pkg, "copyright"))
	if err != nil {
		return err
	}
	licenses := filepath.Join(bundle, "licenses")
	if err := os.MkdirAll(licenses, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(licenses, pkg+".copyright"), copyright, 0o644)
}
