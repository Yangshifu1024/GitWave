package gitcli

import "os"

type lfsMediaStamp struct {
	info          os.FileInfo
	changeTime    int64
	hasChangeTime bool
}

func stampLFSMedia(file *os.File) (lfsMediaStamp, error) {
	info, err := file.Stat()
	if err != nil {
		return lfsMediaStamp{}, err
	}
	changeTime, known := lfsMediaChangeTime(file, info)
	return lfsMediaStamp{info: info, changeTime: changeTime, hasChangeTime: known}, nil
}

func (before lfsMediaStamp) matches(after lfsMediaStamp) bool {
	return before.hasChangeTime == after.hasChangeTime && before.changeTime == after.changeTime &&
		sameDiscardMetadata(before.info, after.info)
}
