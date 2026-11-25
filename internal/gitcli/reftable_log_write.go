package gitcli

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"slices"
	"strings"
)

// Git orders records by ref name and then by decreasing transaction index.
// Copy the payload verbatim so identity, time zone and message survive.
type reftableLogCopy struct {
	name  string
	index uint64
	raw   []byte
}

const reftableLogBlockBudget = 256 << 10

func writeReftableLogCopies(copies []reftableLogCopy, nextIndex uint64, oidSize int,
	write func(string, []byte) error) error {
	slices.SortFunc(copies, func(a, b reftableLogCopy) int {
		if a.index < b.index {
			return -1
		}
		if a.index > b.index {
			return 1
		}
		return strings.Compare(a.name, b.name)
	})
	if uint64(len(copies)) > ^uint64(0)-nextIndex {
		return fmt.Errorf("the Git reflog transaction index is exhausted")
	}
	for i := range copies {
		copies[i].index = nextIndex + uint64(i)
	}
	for len(copies) > 0 {
		n, size := 0, 0
		for _, record := range copies {
			// Reserve room for the full key, its varints and a restart offset.
			size += len(record.name) + len(record.raw) + 31
			if n > 0 && size > reftableLogBlockBudget {
				break
			}
			n++
		}
		chunk := copies[:n]
		min, max := chunk[0].index, chunk[n-1].index
		slices.SortFunc(chunk, compareReftableLogCopies)
		data, err := encodeReftableLogFile(chunk, min, max, oidSize)
		if err != nil {
			return err
		}
		name := fmt.Sprintf("0x%012x-0x%012x-%s.log", min, max, strings.ToLower(rand.Text()))
		if err := write(name, data); err != nil {
			return err
		}
		copies = copies[n:]
	}
	return nil
}

func compareReftableLogCopies(a, b reftableLogCopy) int {
	if order := strings.Compare(a.name, b.name); order != 0 {
		return order
	}
	if a.index > b.index {
		return -1
	}
	if a.index < b.index {
		return 1
	}
	return 0
}

func encodeReftableLogFile(records []reftableLogCopy, min, max uint64, oidSize int) ([]byte, error) {
	header := make([]byte, 24, 28)
	copy(header, "REFT")
	header[4] = 1
	if oidSize == 32 {
		header[4] = 2
		header = append(header, "s256"...)
	}
	binary.BigEndian.PutUint64(header[8:16], min)
	binary.BigEndian.PutUint64(header[16:24], max)

	var data, restarts []byte
	for i, record := range records {
		if i%16 == 0 {
			offset := len(header) + 4 + len(data)
			if offset >= 1<<24 {
				return nil, fmt.Errorf("a Git reflog record exceeds the table block limit")
			}
			restarts = append(restarts, byte(offset>>16), byte(offset>>8), byte(offset))
		}
		// Keys are small compared with log payloads. Keep full keys
		// between restart points as well.
		keySize := len(record.name) + 1 + 8
		data = appendReftableVarint(data, 0)
		data = appendReftableVarint(data, uint64(keySize)<<3|1)
		data = append(data, record.name...)
		data = append(data, 0)
		data = binary.BigEndian.AppendUint64(data, ^record.index)
		data = append(data, record.raw...)
	}
	data = append(data, restarts...)
	data = binary.BigEndian.AppendUint16(data, uint16(len(restarts)/3))
	size := len(header) + 4 + len(data)
	if size >= 1<<24 {
		return nil, fmt.Errorf("a Git reflog record exceeds the table block limit")
	}
	var table bytes.Buffer
	table.Write(header)
	table.Write([]byte{'g', byte(size >> 16), byte(size >> 8), byte(size)})
	zip := zlib.NewWriter(&table)
	if _, err := zip.Write(data); err != nil {
		_ = zip.Close()
		return nil, err
	}
	if err := zip.Close(); err != nil {
		return nil, err
	}
	// A log-only table has no index or object sections. Git treats a zero
	// log offset here as a first block following the file header.
	footer := append(slices.Clone(header), make([]byte, 40)...)
	footer = binary.BigEndian.AppendUint32(footer, crc32.ChecksumIEEE(footer))
	table.Write(footer)
	return table.Bytes(), nil
}

func appendReftableVarint(dst []byte, value uint64) []byte {
	var buf [10]byte
	i := len(buf) - 1
	buf[i] = byte(value & 0x7f)
	for value >>= 7; value > 0; value >>= 7 {
		i--
		value--
		buf[i] = byte(0x80 | value&0x7f)
	}
	return append(dst, buf[i:]...)
}
