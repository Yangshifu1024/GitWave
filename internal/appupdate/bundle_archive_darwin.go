//go:build darwin && !ios

package appupdate

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const macArchiveBundle = "GitWave.app"

type macArchiveEntry struct {
	file *zip.File
	link string
}

// rewriteMacArchive keeps AppleDouble metadata for ditto. Canonical ZIP
// headers make the extractor use the same paths that we validated here.
func rewriteMacArchive(ctx context.Context, input, output *os.File) error {
	info, err := input.Stat()
	if err != nil {
		return err
	}
	archive, err := zip.NewReader(input, info.Size())
	if err != nil {
		return ErrPackage
	}
	if err := validateMacArchive(archive.File); err != nil {
		return err
	}
	writer := zip.NewWriter(output)
	buffer := make([]byte, 64*1024)
	for _, file := range archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		header := file.FileHeader
		header.Extra = nil
		header.Comment = ""
		header.NonUTF8 = false
		header.Flags |= 0x800 // The validated names are UTF-8, even when the input omitted the flag.
		if file.Mode().IsDir() {
			if _, err := writer.CreateHeader(&header); err != nil {
				return err
			}
			continue
		}
		if err := verifyMacArchivePayload(ctx, file, buffer); err != nil {
			return err
		}
		entry, err := writer.CreateRaw(&header)
		if err != nil {
			return err
		}
		raw, err := file.OpenRaw()
		if err != nil {
			return err
		}
		if _, err := io.Copy(entry, raw); err != nil {
			return err
		}
	}
	return writer.Close()
}

func verifyMacArchivePayload(ctx context.Context, file *zip.File, buffer []byte) error {
	reader, err := file.Open()
	if err != nil {
		return errors.Join(ErrPackage, err)
	}
	defer reader.Close()
	var checksum uint32
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		count, err := reader.Read(buffer)
		checksum = crc32.Update(checksum, crc32.IEEETable, buffer[:count])
		if err == io.EOF {
			if checksum != file.CRC32 {
				return errors.Join(ErrPackage, zip.ErrChecksum)
			}
			return nil
		}
		if err != nil {
			return errors.Join(ErrPackage, err)
		}
	}
}

func validateMacArchive(files []*zip.File) error {
	if len(files) == 0 || len(files) > 16000 {
		return ErrPackage
	}
	entries := make(map[string]macArchiveEntry, len(files))
	key := macArchiveKey()
	var total uint64
	for _, file := range files {
		name := strings.TrimSuffix(file.Name, "/")
		metadata := name == "__MACOSX" || name == "__MACOSX/"+macArchiveBundle || strings.HasPrefix(name, "__MACOSX/"+macArchiveBundle+"/") || name == "__MACOSX/._"+macArchiveBundle
		if !validMacArchivePath(name) || (!metadata && name != macArchiveBundle && !strings.HasPrefix(name, macArchiveBundle+"/")) {
			return ErrPackage
		}
		if _, duplicate := entries[key(name)]; duplicate || file.UncompressedSize64 > (2<<30)-total {
			return ErrPackage
		}
		total += file.UncompressedSize64
		mode := file.Mode()
		if file.Flags & ^uint16(0x080e) != 0 || mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
			return ErrPackage
		}
		entry := macArchiveEntry{file: file}
		switch {
		case mode.IsDir():
			if file.UncompressedSize64 != 0 {
				return ErrPackage
			}
		case name == macArchiveBundle || name == "__MACOSX" || name == "__MACOSX/"+macArchiveBundle:
			return ErrPackage
		case mode&os.ModeSymlink != 0 && !metadata:
			link, err := readMacArchiveLink(file)
			if err != nil {
				return err
			}
			entry.link = link
		case mode.IsRegular():
		default:
			return ErrPackage
		}
		entries[key(name)] = entry
	}
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.file.Name, "/")
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if container, ok := entries[key(parent)]; ok && !container.file.Mode().IsDir() {
				return ErrPackage
			}
		}
		if entry.link != "" && !linkStaysInMacBundle(path.Dir(name), entry.link, entries, key) {
			return ErrPackage
		}
	}
	return nil
}

func macArchiveKey() func(string) string {
	fold := cases.Fold()
	return func(name string) string { return fold.String(norm.NFD.String(name)) }
}

func validMacArchivePath(name string) bool {
	return validMacArchiveString(name) && name != "." && path.Clean(name) == name && name != ".." && !strings.HasPrefix(name, "../")
}

func validMacArchiveString(name string) bool {
	return name != "" && len(name) <= 1024 && utf8.ValidString(name) && !path.IsAbs(name) && !strings.ContainsFunc(name, func(r rune) bool {
		// HFS+ ignores these format characters when it compares path components.
		return r == '\\' || r == ':' || r == 0 || r >= 0x200c && r <= 0x200f ||
			r >= 0x202a && r <= 0x202e || r >= 0x206a && r <= 0x206f || r == 0xfeff
	})
}

func readMacArchiveLink(file *zip.File) (string, error) {
	if file.UncompressedSize64 > 1024 {
		return "", ErrPackage
	}
	reader, err := file.Open()
	if err != nil {
		return "", errors.Join(ErrPackage, err)
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, 1025))
	if err != nil {
		return "", errors.Join(ErrPackage, err)
	}
	link := string(data)
	if !validMacArchiveString(link) {
		return "", fmt.Errorf("%w: invalid symbolic link", ErrPackage)
	}
	return link, nil
}

func linkStaysInMacBundle(parent, target string, entries map[string]macArchiveEntry, key func(string) string) bool {
	parts := strings.Split(parent, "/")
	remaining := strings.Split(target, "/")
	hops := 0
	for len(remaining) != 0 {
		part := remaining[0]
		remaining = remaining[1:]
		switch part {
		case "", ".":
		case "..":
			if len(parts) == 1 {
				return false
			}
			parts = parts[:len(parts)-1]
		default:
			name := strings.Join(parts, "/") + "/" + part
			if entry := entries[key(name)]; entry.link != "" {
				hops++
				if hops > 32 {
					return false
				}
				remaining = append(strings.Split(entry.link, "/"), remaining...)
			} else {
				parts = append(parts, part)
			}
		}
	}
	return true
}
