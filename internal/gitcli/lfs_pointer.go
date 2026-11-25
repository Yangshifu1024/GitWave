package gitcli

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const lfsPointerLimit = 1024
const lfsPointerVersion = "https://git-lfs.github.com/spec/v1"

type lfsPointer struct {
	oid      string
	size     int64
	extended bool
}

func (p lfsPointer) writeTo(w io.Writer) error {
	_, err := fmt.Fprintf(w, "version %s\noid sha256:%s\nsize %d\n", lfsPointerVersion, p.oid, p.size)
	return err
}

func parseLFSPointer(data []byte) (lfsPointer, bool) {
	var pointer lfsPointer
	if len(data) == 0 || len(data) >= lfsPointerLimit {
		return pointer, false
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	if len(lines) < 3 {
		return pointer, false
	}
	switch strings.TrimSuffix(string(lines[0]), "\r") {
	case "version " + lfsPointerVersion, "version https://hawser.github.com/spec/v1", "version http://git-media.io/v/2":
	default:
		return pointer, false
	}
	oid, ok := strings.CutPrefix(strings.TrimSuffix(string(lines[len(lines)-2]), "\r"), "oid sha256:")
	if !ok || !validLFSOID(oid) {
		return pointer, false
	}
	size, ok := strings.CutPrefix(strings.TrimSuffix(string(lines[len(lines)-1]), "\r"), "size ")
	length, err := strconv.ParseInt(size, 10, 64)
	if !ok || err != nil || length < 0 {
		return pointer, false
	}
	priorities := make(map[int]bool)
	for _, line := range lines[1 : len(lines)-2] {
		key, value, ok := strings.Cut(strings.TrimSuffix(string(line), "\r"), " ")
		extension, isExtension := strings.CutPrefix(key, "ext-")
		priority, name, validName := strings.Cut(extension, "-")
		extOID, validOIDType := strings.CutPrefix(value, "sha256:")
		n, err := strconv.Atoi(priority)
		if !ok || !isExtension || !validName || name == "" || err != nil || n < 0 || priorities[n] ||
			!validOIDType || !validLFSOID(extOID) {
			return pointer, false
		}
		priorities[n] = true
	}
	return lfsPointer{oid: oid, size: length, extended: len(priorities) != 0}, true
}

func validLFSOID(oid string) bool {
	return len(oid) == 64 && validOID(oid) && oid == strings.ToLower(oid)
}
