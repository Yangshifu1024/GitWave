package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var errWorkFileTooLarge = errors.New("working file exceeds the preview limit")

type workFileContent struct {
	data  []byte
	mode  os.FileMode
	stamp string
}

func (r *Repository) workFile(ctx context.Context, path string, limit int64) (workFileContent, error) {
	local, err := filepath.Localize(path)
	if err != nil || !validFilePath(path) || strings.EqualFold(strings.SplitN(path, "/", 2)[0], ".git") {
		return workFileContent{}, fmt.Errorf("invalid working file path")
	}
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return workFileContent{}, err
	}
	defer root.Close()
	return readWorkFile(ctx, root, local, limit)
}

func readWorkFile(ctx context.Context, root *os.Root, local string, limit int64) (workFileContent, error) {
	info, err := root.Lstat(local)
	if err != nil {
		return workFileContent{}, err
	}
	return readWorkFileWithInfo(ctx, root, local, limit, info)
}

func readWorkFileWithInfo(ctx context.Context, root *os.Root, local string, limit int64, info os.FileInfo) (workFileContent, error) {
	if info.Size() > limit {
		return workFileMetadata(info), errWorkFileTooLarge
	}
	if info.Mode()&os.ModeSymlink != 0 {
		content := workFileMetadata(info)
		destination, err := root.Readlink(local)
		if int64(len(destination)) > limit {
			return content, errWorkFileTooLarge
		}
		content.data = []byte(destination)
		return content, err
	}
	if !info.Mode().IsRegular() {
		return workFileContent{}, fmt.Errorf("working file is not a regular file")
	}
	file, err := root.OpenFile(local, os.O_RDONLY|workFileOpenFlags, 0)
	if err != nil {
		return workFileContent{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return workFileContent{}, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return workFileContent{}, fmt.Errorf("working file changed while opening it")
	}
	content := workFileMetadata(opened)
	// Size the buffer from the open file, but keep the read bounded if it grows.
	var dataBuffer bytes.Buffer
	dataBuffer.Grow(int(min(opened.Size(), limit)) + bytes.MinRead)
	_, err = dataBuffer.ReadFrom(io.LimitReader(file, limit+1))
	data := dataBuffer.Bytes()
	if int64(len(data)) > limit {
		return content, errWorkFileTooLarge
	}
	if ctx.Err() != nil {
		return content, ctx.Err()
	}
	content.data = data
	return content, err
}

func workFileMetadata(info os.FileInfo) workFileContent {
	return workFileContent{mode: info.Mode(), stamp: fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())}
}

func (r *Repository) WorkingImage(ctx context.Context, path string) ([]byte, error) {
	content, err := r.workFile(ctx, path, MaxImageBytes)
	return content.data, err
}
