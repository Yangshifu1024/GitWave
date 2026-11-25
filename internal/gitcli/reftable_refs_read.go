package gitcli

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"os"
	"strings"
)

type reftableRefLayout struct {
	headerSize int64
	oidSize    int
	blockSize  int64
	refsEnd    int64
	refIndex   int64
	maxIndex   uint64
}

func readReftableRefLayout(file *os.File, size int64) (reftableRefLayout, error) {
	header := make([]byte, 28)
	if _, err := file.ReadAt(header, 0); err != nil {
		return reftableRefLayout{}, err
	}
	layout := reftableRefLayout{
		headerSize: 24, oidSize: 20, blockSize: int64(reftableUint24(header[5:8])),
		maxIndex: binary.BigEndian.Uint64(header[16:24]),
	}
	if string(header[:4]) != "REFT" {
		return reftableRefLayout{}, fmt.Errorf("invalid Git reference table")
	}
	switch header[4] {
	case 1:
	case 2:
		layout.headerSize = 28
		switch string(header[24:28]) {
		case "s256":
			layout.oidSize = 32
		case "sha1":
		default:
			return reftableRefLayout{}, fmt.Errorf("unsupported Git table object format")
		}
	default:
		return reftableRefLayout{}, fmt.Errorf("unsupported Git table version")
	}
	footer := make([]byte, layout.headerSize+44)
	footerOffset := size - int64(len(footer))
	if footerOffset < layout.headerSize {
		return reftableRefLayout{}, fmt.Errorf("invalid Git table size")
	}
	if _, err := file.ReadAt(footer, footerOffset); err != nil {
		return reftableRefLayout{}, err
	}
	checksum := binary.BigEndian.Uint32(footer[len(footer)-4:])
	if !bytes.Equal(header[:layout.headerSize], footer[:layout.headerSize]) ||
		crc32.ChecksumIEEE(footer[:len(footer)-4]) != checksum {
		return reftableRefLayout{}, fmt.Errorf("invalid Git reference table footer")
	}
	layout.refsEnd = footerOffset
	for i := 0; i < 5; i++ {
		offset := binary.BigEndian.Uint64(footer[layout.headerSize+int64(i*8):])
		if i == 1 {
			offset >>= 5 // Object offset shares the field with object ID length.
		}
		if offset != 0 {
			if offset < uint64(layout.headerSize) || offset > uint64(footerOffset) {
				return reftableRefLayout{}, fmt.Errorf("invalid Git reference table section offset")
			}
			if i == 0 {
				layout.refIndex = int64(offset)
			}
			layout.refsEnd = min(layout.refsEnd, int64(offset))
		}
	}
	return layout, nil
}

func reftableUint24(data []byte) int {
	return int(data[0])<<16 | int(data[1])<<8 | int(data[2])
}

// Git's public ref iterator omits dangling symbolic refs. Read the ref
// blocks of the locked snapshot so aliases and ref deletions are retained.
func readReftableRefStack(ctx context.Context, root *os.Root, tables []string, from, to string) ([]remoteTrackingRef, int, uint64, error) {
	refs := make(map[string]remoteTrackingRef)
	var maxIndex uint64
	var oidSize int
	for _, name := range tables {
		layout, err := readReftableRefFile(ctx, root, name, func(ref remoteTrackingRef) {
			delete(refs, ref.name)
			if strings.HasPrefix(ref.name, "refs/") && (ref.symbolic != "" ||
				ref.oid != "" && (strings.HasPrefix(ref.name, from) || strings.HasPrefix(ref.name, to))) {
				refs[ref.name] = ref
			}
		})
		if err != nil {
			return nil, 0, 0, fmt.Errorf("read Git reference table %q: %w", name, err)
		}
		if oidSize != 0 && oidSize != layout.oidSize {
			return nil, 0, 0, fmt.Errorf("inconsistent Git reference table object formats")
		}
		oidSize = layout.oidSize
		maxIndex = max(maxIndex, layout.maxIndex)
	}
	items := make([]remoteTrackingRef, 0, len(refs))
	for _, ref := range refs {
		items = append(items, ref)
	}
	return items, oidSize, maxIndex, nil
}

func readReftableRefFile(ctx context.Context, root *os.Root, name string, visit func(remoteTrackingRef)) (reftableRefLayout, error) {
	file, err := root.OpenFile(name, os.O_RDONLY|workFileOpenFlags, 0)
	if err != nil {
		return reftableRefLayout{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return reftableRefLayout{}, fmt.Errorf("invalid Git reference table file")
	}
	layout, err := readReftableRefLayout(file, info.Size())
	if err != nil {
		return reftableRefLayout{}, err
	}
	for offset := layout.headerSize; offset < layout.refsEnd; {
		if err := ctx.Err(); err != nil {
			return reftableRefLayout{}, err
		}
		var header [4]byte
		if _, err := file.ReadAt(header[:], offset); err != nil {
			return reftableRefLayout{}, err
		}
		if header[0] == 'g' && offset == layout.headerSize {
			break // A log-only table has no ref blocks.
		}
		length := int64(reftableUint24(header[1:]))
		if header[0] == 'i' && offset > layout.headerSize && offset < layout.refIndex &&
			length >= 6 && length <= layout.refsEnd-offset {
			break // Lower levels of the ref index precede the root index.
		}
		if header[0] != 'r' {
			return reftableRefLayout{}, fmt.Errorf("invalid Git reference block")
		}
		if offset == layout.headerSize {
			length -= layout.headerSize
		}
		if length < 10 || offset+length > layout.refsEnd {
			return reftableRefLayout{}, fmt.Errorf("invalid Git reference block size")
		}
		data := make([]byte, length-4)
		if _, err := file.ReadAt(data, offset+4); err != nil {
			return reftableRefLayout{}, err
		}
		if err := decodeReftableRefBlock(data, layout.oidSize, visit); err != nil {
			return reftableRefLayout{}, err
		}
		offset += length
		if layout.blockSize != 0 && offset < layout.refsEnd {
			offset = (offset + layout.blockSize - 1) / layout.blockSize * layout.blockSize
		}
	}
	return layout, nil
}

func decodeReftableRefBlock(data []byte, oidSize int, visit func(remoteTrackingRef)) error {
	restartCount := int(binary.BigEndian.Uint16(data[len(data)-2:]))
	recordEnd := len(data) - 2 - 3*restartCount
	if restartCount == 0 || recordEnd <= 0 {
		return fmt.Errorf("invalid Git reference block restart table")
	}
	d := logTableDecoder{rest: data[:recordEnd]}
	var key []byte
	for len(d.rest) != 0 {
		prefix, flags := d.varint(), d.varint()
		suffix := d.take(flags >> 3)
		if d.invalid || prefix > uint64(len(key)) {
			return fmt.Errorf("invalid Git reference key")
		}
		key = append(key[:prefix], suffix...)
		_ = d.varint() // Update index relative to the table minimum.
		ref := remoteTrackingRef{name: string(key)}
		switch flags & 7 {
		case 0:
		case 1, 2:
			ref.oid = hex.EncodeToString(d.take(uint64(oidSize)))
			if flags&7 == 2 {
				_ = d.take(uint64(oidSize)) // Peeled object ID.
			}
		case 3:
			ref.symbolic = string(d.take(d.varint()))
		default:
			return fmt.Errorf("unknown Git reference type")
		}
		if d.invalid {
			return fmt.Errorf("invalid Git reference data")
		}
		visit(ref)
	}
	return nil
}
