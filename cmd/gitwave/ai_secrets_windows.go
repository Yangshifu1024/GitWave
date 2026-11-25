package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	legacyCredentialGeneric = 1
	legacyCredentialMaxSize = 5 * 512
)

var (
	legacyAdvapi     = windows.NewLazySystemDLL("advapi32.dll")
	legacyCredRead   = legacyAdvapi.NewProc("CredReadW")
	legacyCredDelete = legacyAdvapi.NewProc("CredDeleteW")
	legacyCredFree   = legacyAdvapi.NewProc("CredFree")
)

// CREDENTIALW layout used by CredReadW.
type legacyCredential struct {
	Flags, Type uint32
	TargetName  *uint16
	Comment     *uint16
	LastWritten windows.Filetime
	BlobSize    uint32
	Blob        *byte
	Persist     uint32
	AttributesN uint32
	Attributes  uintptr
	TargetAlias *uint16
	UserName    *uint16
}

func readLegacySecret(ctx context.Context, service, account string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	// The Windows backend of Rust keyring 3.x names the target
	// account.service and stores passwords as UTF-16LE.
	target, err := windows.UTF16PtrFromString(account + "." + service)
	if err != nil {
		return "", false, err
	}
	var item *legacyCredential
	ok, _, err := legacyCredRead.Call(uintptr(unsafe.Pointer(target)), legacyCredentialGeneric, 0, uintptr(unsafe.Pointer(&item)))
	if ok == 0 {
		if errors.Is(err, windows.ERROR_NOT_FOUND) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read legacy key: %w", err)
	}
	defer legacyCredFree.Call(uintptr(unsafe.Pointer(item)))
	if item == nil || item.BlobSize > legacyCredentialMaxSize || item.BlobSize%2 != 0 || (item.BlobSize != 0 && item.Blob == nil) {
		return "", false, errors.New("legacy key has invalid encoding")
	}
	raw := unsafe.Slice(item.Blob, item.BlobSize)
	codeUnits := make([]uint16, len(raw)/2)
	for i := range codeUnits {
		codeUnits[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	return string(utf16.Decode(codeUnits)), true, nil
}

func deleteLegacySecret(ctx context.Context, service, account string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(account + "." + service)
	if err != nil {
		return err
	}
	ok, _, err := legacyCredDelete.Call(uintptr(unsafe.Pointer(target)), legacyCredentialGeneric, 0)
	if ok == 0 && !errors.Is(err, windows.ERROR_NOT_FOUND) {
		return fmt.Errorf("delete legacy key: %w", err)
	}
	return nil
}
