package main

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
	"unsafe"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

func TestReadAndDeleteLegacyWindowsAIKey(t *testing.T) {
	account := uuid.NewString() + ":openai"
	ctx := t.Context()
	target, err := windows.UTF16PtrFromString(account + "." + aiSecretService)
	if err != nil {
		t.Fatal(err)
	}
	const value = "fixture-key-中文"
	units := utf16.Encode([]rune(value))
	password := make([]byte, 2*len(units))
	for i, unit := range units {
		binary.LittleEndian.PutUint16(password[i*2:], unit)
	}
	item := legacyCredential{TargetName: target, Type: legacyCredentialGeneric,
		Blob: &password[0], BlobSize: uint32(len(password)), Persist: 1}
	write := legacyAdvapi.NewProc("CredWriteW")
	if ok, _, err := write.Call(uintptr(unsafe.Pointer(&item)), 0); ok == 0 {
		t.Fatalf("write temporary Windows fixture: %v", err)
	}
	defer func() {
		if err := deleteLegacySecret(ctx, aiSecretService, account); err != nil {
			t.Error(err)
		}
	}()
	found, exists, err := readLegacySecret(ctx, aiSecretService, account)
	if err != nil || !exists || found != value {
		t.Fatalf("legacy read: exists = %v, err = %v", exists, err)
	}
	if err := deleteLegacySecret(ctx, aiSecretService, account); err != nil {
		t.Fatal(err)
	}
	if _, exists, err = readLegacySecret(ctx, aiSecretService, account); err != nil || exists {
		t.Fatalf("legacy delete: exists = %v, err = %v", exists, err)
	}
}
