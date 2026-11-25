package gitcli

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"math"
	"os"
	"slices"
)

// Reftable updates may replace or tombstone an earlier log record. Read
// immutable tables oldest first and keep the newest record for each key.
// See https://git-scm.com/docs/reftable#_log_block_format.
func readReftableReflog(ctx context.Context, root *os.Root, ref string) ([]ReflogEntry, error) {
	stack, err := root.OpenRoot("reftable")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer stack.Close()
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		list, _, err := readGitStorageFile(stack, "tables.list", 8<<20)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		files, err := parseReftableList(list)
		if err != nil {
			return nil, err
		}
		entries, readErr := readReftableLogStack(ctx, stack, files, ref)
		current, _, err := readGitStorageFile(stack, "tables.list", 8<<20)
		if err != nil {
			return nil, err
		}
		if bytes.Equal(list, current) {
			return entries, readErr
		}
		// A concurrent ref transaction can retire tables while we read.
		// Retry against a fresh manifest rather than showing a partial log.
	}
	return nil, fmt.Errorf("the Git reference tables changed while reading the reflog")
}

func readReftableLogStack(ctx context.Context, root *os.Root, tables []string, ref string) ([]ReflogEntry, error) {
	records := make(map[uint64]ReflogEntry)
	for _, table := range tables {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := readReftableLogFile(ctx, root, table, func(record reftableLogRecord) {
			if record.name != ref {
				return
			}
			if record.raw == nil {
				delete(records, record.index)
			} else {
				records[record.index] = newReflogEntry(hex.EncodeToString(record.old), hex.EncodeToString(record.next),
					string(record.message), string(record.committer), int64(record.seconds))
			}
		}); err != nil {
			return nil, fmt.Errorf("read Git log table %q: %w", table, err)
		}
	}
	order := make([]uint64, 0, len(records))
	for index := range records {
		order = append(order, index)
	}
	slices.Sort(order)
	slices.Reverse(order)
	result := make([]ReflogEntry, 0, len(records))
	for _, index := range order {
		result = append(result, records[index])
	}
	return result, nil
}

type reftableLogRecord struct {
	name                               string
	index, seconds                     uint64
	raw, old, next, committer, message []byte
}

func readReftableLogFile(ctx context.Context, root *os.Root, name string, visit func(reftableLogRecord)) error {
	pathInfo, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !pathInfo.Mode().IsRegular() {
		return fmt.Errorf("invalid Git table file")
	}
	file, err := root.OpenFile(name, os.O_RDONLY|workFileOpenFlags, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !sameDiscardMetadata(pathInfo, info) {
		return fmt.Errorf("Git table changed while opening it")
	}
	header := make([]byte, 28)
	if _, err := file.ReadAt(header, 0); err != nil {
		return err
	}
	headerSize, oidSize := 24, 20
	if string(header[:4]) != "REFT" {
		return fmt.Errorf("invalid Git table header")
	}
	switch header[4] {
	case 1:
	case 2:
		headerSize = 28
		switch string(header[24:28]) {
		case "sha1":
		case "s256":
			oidSize = 32
		default:
			return fmt.Errorf("unknown Git table object format")
		}
	default:
		return fmt.Errorf("unsupported Git table version %d", header[4])
	}
	footer := make([]byte, headerSize+44)
	footerOffset := info.Size() - int64(len(footer))
	if footerOffset < int64(headerSize) {
		return fmt.Errorf("invalid Git table size")
	}
	if _, err := file.ReadAt(footer, footerOffset); err != nil {
		return err
	}
	checksum := binary.BigEndian.Uint32(footer[len(footer)-4:])
	if !bytes.Equal(header[:headerSize], footer[:headerSize]) || crc32.ChecksumIEEE(footer[:len(footer)-4]) != checksum {
		return fmt.Errorf("invalid Git table footer")
	}
	position := binary.BigEndian.Uint64(footer[headerSize+24:])
	if position == 0 {
		if footerOffset == int64(headerSize) {
			return nil
		}
		var kind [1]byte
		if _, err := file.ReadAt(kind[:], int64(headerSize)); err != nil {
			return err
		}
		if kind[0] != 'g' {
			return nil
		}
		// Git writes offset zero for a log-only table because its first
		// block includes the file header in the logical block length.
		position = uint64(headerSize)
	}
	logIndex := binary.BigEndian.Uint64(footer[headerSize+32:])
	end := logIndex
	if end == 0 {
		end = uint64(footerOffset)
	}
	if position < uint64(headerSize) || position >= end || end > uint64(footerOffset) {
		return fmt.Errorf("invalid Git table log range")
	}
	reader := io.NewSectionReader(file, int64(position), int64(end-position))
	// zlib reads bytewise at the boundary when its input implements
	// io.ByteReader, so the next block starts at the next byte.
	stream := bufio.NewReader(reader)
	for position < end {
		if err := ctx.Err(); err != nil {
			return err
		}
		var block [4]byte
		if _, err := io.ReadFull(stream, block[:]); err != nil {
			return err
		}
		size := int(block[1])<<16 | int(block[2])<<8 | int(block[3])
		if position == uint64(headerSize) {
			size -= headerSize
		}
		if block[0] == 'i' && logIndex != 0 && size >= 6 && uint64(size) <= end-position {
			// The footer points at the top index level. Lower-level index
			// blocks follow the log blocks and may appear before that offset.
			return nil
		}
		if block[0] != 'g' || size < 6 {
			return fmt.Errorf("invalid Git table log block")
		}
		inflater, err := zlib.NewReader(stream)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(inflater, int64(size-3)))
		closeErr := inflater.Close()
		if err != nil || closeErr != nil || len(data) != size-4 {
			return errors.Join(err, closeErr, fmt.Errorf("invalid compressed Git log block"))
		}
		if err := decodeReftableLogBlock(data, oidSize, visit); err != nil {
			return err
		}
		offset, _ := reader.Seek(0, io.SeekCurrent)
		position = end - uint64(reader.Size()-offset+int64(stream.Buffered()))
	}
	return nil
}

func decodeReftableLogBlock(block []byte, oidSize int, visit func(reftableLogRecord)) error {
	restartCount := int(binary.BigEndian.Uint16(block[len(block)-2:]))
	recordEnd := len(block) - 2 - 3*restartCount
	if restartCount == 0 || recordEnd <= 0 {
		return fmt.Errorf("invalid Git log block restart table")
	}
	d := logTableDecoder{rest: block[:recordEnd]}
	var key []byte
	for len(d.rest) != 0 && !d.invalid {
		prefixLen, flags := d.varint(), d.varint()
		suffix := d.take(flags >> 3)
		if d.invalid || prefixLen > uint64(len(key)) {
			return fmt.Errorf("invalid Git log key")
		}
		key = append(key[:prefixLen], suffix...)
		keyName, reverseIndex, found := bytes.Cut(key, []byte{0})
		if !found || len(reverseIndex) != 8 {
			return fmt.Errorf("invalid Git log record name")
		}
		index := ^binary.BigEndian.Uint64(reverseIndex)
		record := reftableLogRecord{name: string(keyName), index: index}
		switch flags & 7 {
		case 0:
		case 1:
			data := d.rest
			record.old, record.next = d.take(uint64(oidSize)), d.take(uint64(oidSize))
			record.committer = d.take(d.varint())
			_ = d.take(d.varint()) // email
			record.seconds = d.varint()
			_ = d.take(2) // timezone
			record.message = d.take(d.varint())
			if record.seconds > math.MaxInt64 || d.invalid {
				return fmt.Errorf("invalid Git log record data")
			}
			record.raw = data[:len(data)-len(d.rest)]
		default:
			return fmt.Errorf("unknown Git log record type")
		}
		visit(record)
	}
	if d.invalid {
		return fmt.Errorf("invalid compressed Git log record")
	}
	return nil
}

type logTableDecoder struct {
	rest    []byte
	invalid bool
}

func (d *logTableDecoder) take(count uint64) []byte {
	if count > uint64(len(d.rest)) {
		d.invalid = true
		return nil
	}
	value := d.rest[:count]
	d.rest = d.rest[count:]
	return value
}

func (d *logTableDecoder) varint() uint64 {
	var value uint64
	for len(d.rest) != 0 && !d.invalid {
		next := d.rest[0]
		d.rest = d.rest[1:]
		value |= uint64(next & 0x7f)
		if next&0x80 == 0 {
			return value
		}
		if value >= math.MaxUint64>>7 {
			break
		}
		value = (value + 1) << 7
	}
	d.invalid = true
	return 0
}
