package main

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"gitwave.work/internal/desktopgtk"
	"gitwave.work/internal/distasset"
)

const gtkEnvironment = "GITWAVE_PACKAGING_GTK_ROOT"
const gtkSourcesDirectory = "gtk-source"

//go:embed linux-gtk-build.sh
var gtkBuildScript []byte

const ubuntuPool = "https://archive.ubuntu.com/ubuntu/pool/main/"

var gtkSources = []struct{ pool, name, sha string }{
	{"g/glib2.0/", "glib2.0_2.80.0-6ubuntu3.10.dsc", "0ee18e055f5fba9a33763f94823b44ab38e4f677f625ba7540486fe03b436136"},
	{"g/glib2.0/", "glib2.0_2.80.0-6ubuntu3.10.debian.tar.xz", "9790a8bbf6d775f8a482f3862a64b2127330a1866ceaf04c85be34fbd2698254"},
	{"g/glib2.0/", "glib2.0_2.80.0.orig.tar.xz", "8228a92f92a412160b139ae68b6345bd28f24434a7b5af150ebe21ff587a561d"},
	{"g/glib2.0/", "glib2.0_2.80.0.orig-unicode-data.tar.xz", "38680f78a0ae6258826418cb5096c19ae3566ba8fee0a2112a0ec40056e58729"},
	{"g/gtk4/", "gtk4_4.14.5+ds-0ubuntu0.10.dsc", "04ace1e47b9d3a94c876f34c10b0e7d8507dc58e5517e8951f30932afcb12126"},
	{"g/gtk4/", "gtk4_4.14.5+ds-0ubuntu0.10.debian.tar.xz", "17692a86b96fa9bc930f744694c02a43f7a048abdfb1f9864fe2b9925f649d00"},
	{"g/gtk4/", "gtk4_4.14.5+ds.orig.tar.xz", "bd52b294cd18784339b105e2f25d2569e3db94411c64b9b141b23fe39118a0e5"},
	{"w/wayland/", "wayland_1.22.0-2.1build1.dsc", "f462ebb5625fc0cdf1c960bb5773099b63fa2765dcc41ec4a66cde33731f30e0"},
	{"w/wayland/", "wayland_1.22.0-2.1build1.diff.gz", "3a22976ff3f1d80b9c7bdebcf0667f2ca4b30f3c2feaaa7e358b2e307a7d082e"},
	{"w/wayland/", "wayland_1.22.0.orig.tar.gz", "b1b023a31902286e476e109ed48070a815d3ab5f615daec29e9635188881eece"},
	{"w/wayland-protocols/", "wayland-protocols_1.45.orig.tar.xz", "4d2b2a9e3e099d017dc8107bf1c334d27bb87d9e4aff19a0c8d856d17cd41ef0"},
}

func buildGTKBundle(work, assets string) (string, error) {
	sources := filepath.Join(work, gtkSourcesDirectory)
	if err := os.Mkdir(sources, 0o755); err != nil {
		return "", err
	}
	var provenance, digests strings.Builder
	provenance.WriteString("Ubuntu sources for the GitWave Linux desktop runtime\n")
	for _, source := range gtkSources {
		asset := distasset.Asset{Name: source.name, URL: ubuntuPool + source.pool + source.name, SHA256: source.sha}
		if _, err := writeAsset(asset, sources, assets); err != nil {
			return "", err
		}
		provenance.WriteString(asset.Provenance())
		digests.WriteString(asset.SHA256 + "  " + asset.Name + "\n")
	}
	if err := os.WriteFile(filepath.Join(sources, "SHA256SUMS"), []byte(digests.String()), 0o644); err != nil {
		return "", err
	}
	script := filepath.Join(sources, "build.sh")
	if err := os.WriteFile(script, gtkBuildScript, 0o644); err != nil {
		return "", err
	}
	bundle := filepath.Join(work, desktopgtk.Directory)
	if err := run(exec.Command("bash", script, bundle, strconv.Itoa(min(8, runtime.GOMAXPROCS(0))))); err != nil {
		return "", err
	}
	return bundle, os.WriteFile(filepath.Join(bundle, "GitWave-third-party-sources.txt"), []byte(provenance.String()), 0o644)
}

func publishGTKSource(work, output string) error {
	source := filepath.Join(work, gtkSourcesDirectory)
	destination := filepath.Join(output, gtkSourcesDirectory)
	files, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	for _, entry := range files {
		data, err := os.ReadFile(filepath.Join(source, entry.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
