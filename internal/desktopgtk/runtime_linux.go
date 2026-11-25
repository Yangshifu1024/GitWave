package desktopgtk

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ebitengine/purego"
)

const probeArg = "--gitwave-probe-system-gtk"
const gtkLibrary = "libgtk-4.so.1"

// Probe runs in a child process so an old system GTK and its dependencies
// never enter the process that may need to load the private runtime.
func Probe() {
	if len(os.Args) != 2 || os.Args[1] != probeArg {
		return
	}
	gtk, err := purego.Dlopen(gtkLibrary, purego.RTLD_LOCAL|purego.RTLD_NOW)
	if err != nil || !compatible(gtk) {
		os.Exit(1)
	}
	os.Exit(0)
}

func Prepare() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err = exec.CommandContext(ctx, executable, probeArg).Run()
	cancel()
	if err == nil {
		return nil
	}
	root := filepath.Join(filepath.Dir(executable), Directory)
	info, err := os.Stat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return errors.New("GTK 4.14 or newer is required and the bundled GTK runtime is missing")
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("invalid GTK runtime directory %s", root)
	}
	schemas := filepath.Join(root, "share", "glib-2.0", "schemas")
	info, err = os.Stat(filepath.Join(schemas, "gschemas.compiled"))
	if err != nil {
		return fmt.Errorf("load the bundled GTK schemas: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("invalid bundled GTK schemas")
	}
	gtk, err := purego.Dlopen(filepath.Join(root, "lib", gtkLibrary), purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		return fmt.Errorf("load the bundled GTK runtime: %w", err)
	}
	if !compatible(gtk) {
		return errors.New("the bundled GTK runtime is older than GTK 4.14 or incompatible")
	}
	glib, err := purego.Dlopen(filepath.Join(root, "lib", "libglib-2.0.so.0"), purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		return fmt.Errorf("load the bundled GLib runtime: %w", err)
	}
	setenv, err := purego.Dlsym(glib, "g_setenv")
	if err != nil {
		return fmt.Errorf("configure the bundled GTK runtime: %w", err)
	}
	var setGTKEnvironment func(string, string, int32) int32
	purego.RegisterFunc(&setGTKEnvironment, setenv)
	// GLib reads the C environment. Setting only that environment keeps
	// desktop paths out of Go's environment for Git, LFS and SSH children.
	if setGTKEnvironment("GSETTINGS_SCHEMA_DIR", schemas, 0) == 0 ||
		setGTKEnvironment("GTK_EXE_PREFIX", root, 0) == 0 {
		return errors.New("configure the bundled GTK runtime paths")
	}
	return nil
}

func compatible(gtk uintptr) bool {
	check, err := purego.Dlsym(gtk, "gtk_check_version")
	if err != nil {
		return false
	}
	var checkVersion func(uint32, uint32, uint32) uintptr
	purego.RegisterFunc(&checkVersion, check)
	return checkVersion(4, 14, 0) == 0
}
