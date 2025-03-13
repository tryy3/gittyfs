package gittyfuse

import (
	"fmt"
	"io/fs"
	"log"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fuse"
)

type SimpleDirStream struct {
	index   int
	entries []fuse.DirEntry
}

func NewSimpleDirStreamFromFiles(files []fs.FileInfo) *SimpleDirStream {
	entries := make([]fuse.DirEntry, len(files))
	for i, file := range files {
		var mode uint32
		if file.IsDir() {
			mode = fuse.S_IFDIR | 0755
		} else {
			mode = fuse.S_IFREG | 0644
		}
		entries[i] = fuse.DirEntry{
			Name: file.Name(),
			Mode: mode,
			Ino:  uint64(i),
		}
	}
	return NewSimpleDirStream(entries)
}

func NewSimpleDirStream(entries []fuse.DirEntry) *SimpleDirStream {
	return &SimpleDirStream{
		index:   0,
		entries: entries,
	}
}

func (s *SimpleDirStream) HasNext() bool {
	log.Printf("HasNext: %d < %d", s.index, len(s.entries))
	return s.index < len(s.entries)
}

func (s *SimpleDirStream) Next() (fuse.DirEntry, syscall.Errno) {
	if !s.HasNext() {
		return fuse.DirEntry{}, ToErrno(fmt.Errorf("no more entries"))
	}
	log.Printf("Next: %d", s.index)
	entry := s.entries[s.index]
	s.index++
	return entry, 0
}

func (s *SimpleDirStream) Close() {
	s.index = len(s.entries)
	log.Printf("Close: %d", s.index)
}
