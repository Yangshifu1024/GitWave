#!/usr/bin/env bash
set -euo pipefail

sources=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
bundle=$1
jobs=$2
build=$(mktemp -d)
trap 'rm -rf "$build"' EXIT
prefix="$build/install"

(cd "$sources" && sha256sum -c SHA256SUMS)
source_descriptor() {
    awk -v prefix="${1}_" 'index($2, prefix) == 1 && $2 ~ /\.dsc$/ { print $2 }' "$sources/SHA256SUMS"
}
dpkg-source -x "$sources/$(source_descriptor glib2.0)" "$build/glib"
dpkg-source -x "$sources/$(source_descriptor wayland)" "$build/wayland"
dpkg-source -x "$sources/$(source_descriptor gtk4)" "$build/gtk"
tar -xf "$sources/wayland-protocols_1.45.orig.tar.xz" -C "$build"

python3 -m venv "$build/meson"
"$build/meson/bin/pip" install --disable-pip-version-check --only-binary=:all: --require-hashes -r /dev/stdin <<'EOF'
meson==1.3.2 --hash=sha256:0ba4a71fbc060c44721c7b674807598c5af9ea51335073cae7a3e9a95b375c89
packaging==24.2 --hash=sha256:09abb1bccd265c01f4a3aa3f7a7db064b36514d2cba19a2f694fe6150451a759
EOF
export PATH="$build/meson/bin:$prefix/bin:$PATH"
export PKG_CONFIG_PATH="$prefix/lib/pkgconfig:$prefix/share/pkgconfig"
export LD_LIBRARY_PATH="$prefix/lib"

install_library() {
    local component=$1
    shift
    meson setup "$build/build-$component" "$build/$component" \
        --prefix="$prefix" --libdir=lib --sysconfdir=/etc --wrap-mode=nodownload \
        -Doptimization=2 -Ddebug=false "$@"
    meson compile -C "$build/build-$component" -j "$jobs"
    meson install -C "$build/build-$component"
}

install_library glib -Dtests=false -Dintrospection=disabled \
    -Dgio_module_dir=/usr/lib/x86_64-linux-gnu/gio/modules
install_library wayland -Ddocumentation=false -Dtests=false
install_library wayland-protocols-1.45 -Dtests=false
install_library gtk -Dwayland-backend=true -Dx11-backend=true -Dvulkan=disabled \
    -Dmedia-gstreamer=disabled -Dbuild-demos=false -Dbuild-tests=false -Dbuild-testsuite=false \
    -Dbuild-examples=false -Dintrospection=disabled

mkdir -p "$bundle/lib" "$bundle/licenses" "$bundle/share/glib-2.0/schemas"
for soname in libgtk-4.so.1 libglib-2.0.so.0 libgobject-2.0.so.0 libgio-2.0.so.0 libgmodule-2.0.so.0 \
    libwayland-client.so.0 libwayland-cursor.so.0 libwayland-egl.so.1; do
    cp -L -- "$prefix/lib/$soname" "$bundle/lib/$soname"
    strip --strip-unneeded "$bundle/lib/$soname"
    patchelf --set-rpath '$ORIGIN' "$bundle/lib/$soname"
done
cp -R "$prefix/lib/gtk-4.0" "$bundle/lib/"
for module in "$bundle"/lib/gtk-4.0/*/printbackends/*.so; do
    strip --strip-unneeded "$module"
    patchelf --set-rpath '$ORIGIN/../../..' "$module"
done
install -m 644 "$prefix/share/glib-2.0/schemas/gschemas.compiled" "$bundle/share/glib-2.0/schemas/"
for component in glib wayland gtk; do
    install -m 644 "$build/$component/debian/copyright" "$bundle/licenses/$component.txt"
    install -m 644 "$build/$component/COPYING" "$bundle/licenses/$component-COPYING.txt"
done
install -m 644 "$build/wayland-protocols-1.45/COPYING" "$bundle/licenses/wayland-protocols.txt"

unset LD_LIBRARY_PATH
links=$(ldd "$bundle/lib/libgtk-4.so.1")
if grep -Fq 'not found' <<< "$links" || ! grep -Fq "$bundle/lib/libglib-2.0.so.0" <<< "$links"; then
    printf '%s\n' "$links" >&2
    exit 1
fi
