// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gittyfuse

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"

	"github.com/go-git/go-billy/v5"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// NewLoopbackFile creates a FileHandle out of a file descriptor. All
// operations are implemented. When using the Fd from a *os.File, call
// syscall.Dup() on the fd, to avoid os.File's finalizer from closing
// the file descriptor.
func NewLoopbackFile(wt billy.Filesystem, path string) fs.FileHandle {
	return &loopbackFile{wt: wt, path: path}
}

type loopbackFile struct {
	mu   sync.Mutex
	path string
	wt   billy.Filesystem
}

var _ = (fs.FileHandle)((*loopbackFile)(nil))
var _ = (fs.FileReleaser)((*loopbackFile)(nil))

// var _ = (fs.FileGetattrer)((*loopbackFile)(nil))
var _ = (fs.FileReader)((*loopbackFile)(nil))
var _ = (fs.FileWriter)((*loopbackFile)(nil))

// var _ = (fs.FileGetlker)((*loopbackFile)(nil))
// var _ = (fs.FileSetlker)((*loopbackFile)(nil))
// var _ = (fs.FileSetlkwer)((*loopbackFile)(nil))
// var _ = (fs.FileLseeker)((*loopbackFile)(nil))
var _ = (fs.FileFlusher)((*loopbackFile)(nil))
var _ = (fs.FileFsyncer)((*loopbackFile)(nil))

// var _ = (fs.FileSetattrer)((*loopbackFile)(nil))
// var _ = (fs.FileAllocater)((*loopbackFile)(nil))

// var _ = (fs.FilePassthroughFder)((*loopbackFile)(nil))

// func (f *loopbackFile) PassthroughFd() (int, bool) {
// 	// This Fd is not accessed concurrently, but lock anyway for uniformity.
// 	f.mu.Lock()
// 	defer f.mu.Unlock()
// 	return f.fd, true
// }

func (f *loopbackFile) Read(ctx context.Context, buf []byte, off int64) (res fuse.ReadResult, errno syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Option 1: If using a Billy filesystem (f.wt)
	file, err := f.wt.Open(f.path)
	if err != nil {
		return nil, ToErrno(fmt.Errorf("failed to open file: %w", err))
	}
	defer file.Close()

	n, err := file.ReadAt(buf, off)
	if err != nil && err != io.EOF {
		return nil, ToErrno(fmt.Errorf("failed to read file: %w", err))
	}

	// Return the actual data that was read
	return fuse.ReadResultData(buf[:n]), OK
}

func (f *loopbackFile) Write(ctx context.Context, data []byte, off int64) (uint32, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Step 1: Read the existing file data
	fileInfo, err := f.wt.Stat(f.path)
	if err != nil && !os.IsNotExist(err) {
		return 0, ToErrno(fmt.Errorf("failed to stat file: %w", err))
	}

	var fileSize int64
	if err == nil {
		fileSize = fileInfo.Size()
	}

	// Create or open the file for reading
	var existingContent []byte
	if fileSize > 0 {
		readFile, err := f.wt.Open(f.path)
		if err != nil {
			return 0, ToErrno(fmt.Errorf("failed to open file for reading: %w", err))
		}

		existingContent = make([]byte, fileSize)
		_, err = readFile.Read(existingContent)
		readFile.Close()

		if err != nil && err != io.EOF {
			return 0, ToErrno(fmt.Errorf("failed to read existing file content: %w", err))
		}
	}

	// Step 2: Calculate the new file size
	requiredSize := off + int64(len(data))
	if requiredSize > fileSize {
		// Extend the existing content if needed
		newContent := make([]byte, requiredSize)
		copy(newContent, existingContent)
		existingContent = newContent
	}

	// Step 3: Modify the data at the specified offset
	copy(existingContent[off:], data)

	// Step 4: Write back the entire content
	writeFile, err := f.wt.OpenFile(f.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return 0, ToErrno(fmt.Errorf("failed to open file for writing: %w", err))
	}
	defer writeFile.Close()

	_, err = writeFile.Write(existingContent)
	if err != nil {
		return 0, ToErrno(fmt.Errorf("failed to write modified content: %w", err))
	}

	// Return only the number of bytes from the original data that were written
	// (even though we wrote the entire file)
	return uint32(len(data)), OK
}

func (f *loopbackFile) Release(ctx context.Context) syscall.Errno {
	//		f.mu.Lock()
	//		defer f.mu.Unlock()
	//		if f.fd != -1 {
	//			err := syscall.Close(f.fd)
	//			f.fd = -1
	//			return ToErrno(err)
	//		}
	//		return syscall.EBADF
	//	}
	return 0
}

func (f *loopbackFile) Flush(ctx context.Context) syscall.Errno {
	// f.mu.Lock()
	// defer f.mu.Unlock()
	// // Since Flush() may be called for each dup'd fd, we don't
	// // want to really close the file, we just want to flush. This
	// // is achieved by closing a dup'd fd.
	// newFd, err := syscall.Dup(f.fd)

	// if err != nil {
	// 	return ToErrno(err)
	// }
	// err = syscall.Close(newFd)
	// return ToErrno(err)
	return 0
}

func (f *loopbackFile) Fsync(ctx context.Context, flags uint32) (errno syscall.Errno) {
	// f.mu.Lock()
	// defer f.mu.Unlock()
	// r := ToErrno(syscall.Fsync(f.fd))

	// return r
	return 0
}

const (
	_OFD_GETLK  = 36
	_OFD_SETLK  = 37
	_OFD_SETLKW = 38
)

// func (f *loopbackFile) Getlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (errno syscall.Errno) {
// 	f.mu.Lock()
// 	defer f.mu.Unlock()
// 	flk := syscall.Flock_t{}
// 	lk.ToFlockT(&flk)
// 	errno = ToErrno(syscall.FcntlFlock(uintptr(f.fd), _OFD_GETLK, &flk))
// 	out.FromFlockT(&flk)
// 	return
// }

// func (f *loopbackFile) Setlk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (errno syscall.Errno) {
// 	return f.setLock(ctx, owner, lk, flags, false)
// }

// func (f *loopbackFile) Setlkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (errno syscall.Errno) {
// 	return f.setLock(ctx, owner, lk, flags, true)
// }

// func (f *loopbackFile) setLock(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, blocking bool) (errno syscall.Errno) {
// 	f.mu.Lock()
// 	defer f.mu.Unlock()
// 	if (flags & fuse.FUSE_LK_FLOCK) != 0 {
// 		var op int
// 		switch lk.Typ {
// 		case syscall.F_RDLCK:
// 			op = syscall.LOCK_SH
// 		case syscall.F_WRLCK:
// 			op = syscall.LOCK_EX
// 		case syscall.F_UNLCK:
// 			op = syscall.LOCK_UN
// 		default:
// 			return syscall.EINVAL
// 		}
// 		if !blocking {
// 			op |= syscall.LOCK_NB
// 		}
// 		return ToErrno(syscall.Flock(f.fd, op))
// 	} else {
// 		flk := syscall.Flock_t{}
// 		lk.ToFlockT(&flk)
// 		var op int
// 		if blocking {
// 			op = _OFD_SETLKW
// 		} else {
// 			op = _OFD_SETLK
// 		}
// 		return ToErrno(syscall.FcntlFlock(uintptr(f.fd), op, &flk))
// 	}
// }

// func (f *loopbackFile) Setattr(ctx context.Context, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
// 	if errno := f.setAttr(ctx, in); errno != 0 {
// 		return errno
// 	}

// 	return f.Getattr(ctx, out)
// }

// func (f *loopbackFile) fchmod(mode uint32) syscall.Errno {
// 	f.mu.Lock()
// 	defer f.mu.Unlock()
// 	return ToErrno(syscall.Fchmod(f.fd, mode))
// }

// func (f *loopbackFile) fchown(uid, gid int) syscall.Errno {
// 	f.mu.Lock()
// 	defer f.mu.Unlock()
// 	return ToErrno(syscall.Fchown(f.fd, uid, gid))
// }

// func (f *loopbackFile) ftruncate(sz uint64) syscall.Errno {
// 	return ToErrno(syscall.Ftruncate(f.fd, int64(sz)))
// }

// func (f *loopbackFile) setAttr(ctx context.Context, in *fuse.SetAttrIn) syscall.Errno {
// 	var errno syscall.Errno
// 	if mode, ok := in.GetMode(); ok {
// 		if errno := f.fchmod(mode); errno != 0 {
// 			return errno
// 		}
// 	}

// 	uid32, uOk := in.GetUID()
// 	gid32, gOk := in.GetGID()
// 	if uOk || gOk {
// 		uid := -1
// 		gid := -1

// 		if uOk {
// 			uid = int(uid32)
// 		}
// 		if gOk {
// 			gid = int(gid32)
// 		}
// 		if errno := f.fchown(uid, gid); errno != 0 {
// 			return errno
// 		}
// 	}

// 	mtime, mok := in.GetMTime()
// 	atime, aok := in.GetATime()

// 	if mok || aok {
// 		ap := &atime
// 		mp := &mtime
// 		if !aok {
// 			ap = nil
// 		}
// 		if !mok {
// 			mp = nil
// 		}
// 		errno = f.utimens(ap, mp)
// 		if errno != 0 {
// 			return errno
// 		}
// 	}

// 	if sz, ok := in.GetSize(); ok {
// 		if errno := f.ftruncate(sz); errno != 0 {
// 			return errno
// 		}
// 	}
// 	return OK
// }

// func (f *loopbackFile) Getattr(ctx context.Context, a *fuse.AttrOut) syscall.Errno {
// 	f.mu.Lock()
// 	defer f.mu.Unlock()
// 	st := syscall.Stat_t{}
// 	err := syscall.Fstat(f.fd, &st)
// 	if err != nil {
// 		return ToErrno(err)
// 	}
// 	a.FromStat(&st)

// 	return OK
// }

// func (f *loopbackFile) Lseek(ctx context.Context, off uint64, whence uint32) (uint64, syscall.Errno) {
// 	f.mu.Lock()
// 	defer f.mu.Unlock()
// 	n, err := unix.Seek(f.fd, int64(off), int(whence))
// 	return uint64(n), ToErrno(err)
// }

// func (f *loopbackFile) Allocate(ctx context.Context, off uint64, sz uint64, mode uint32) syscall.Errno {
// 	f.mu.Lock()
// 	defer f.mu.Unlock()
// 	err := fallocate.Fallocate(f.fd, mode, int64(off), int64(sz))
// 	if err != nil {
// 		return ToErrno(err)
// 	}
// 	return OK
// }
