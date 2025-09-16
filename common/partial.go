package common

import (
	"io/fs"
	"time"
)

type PartialFileInfo struct {
	fileInfo fs.FileInfo
	offset   int64
}

func (p PartialFileInfo) IsDir() bool {
	return p.fileInfo.IsDir()
}

func (p PartialFileInfo) ModTime() time.Time {
	return p.fileInfo.ModTime()
}

func (p PartialFileInfo) Mode() fs.FileMode {
	return p.fileInfo.Mode()
}

func (p PartialFileInfo) Name() string {
	return p.fileInfo.Name()
}

func (p PartialFileInfo) Size() int64 {
	return p.fileInfo.Size() - p.offset
}

func (p PartialFileInfo) Sys() any {
	return p.fileInfo.Sys()
}

func NewPartialFileInfo(fileInfo fs.FileInfo, offset int64) PartialFileInfo {
	return PartialFileInfo{fileInfo, offset}
}
