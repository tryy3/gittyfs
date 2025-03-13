package gittyfuse

import (
	"io/fs"
	"log"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fuse"
)

// OK is the Errno return value to indicate absense of errors.
var OK = syscall.Errno(0)

// RENAME_EXCHANGE is a flag argument for renameat2()
const RENAME_EXCHANGE = 0x2

// seek to the next data
const _SEEK_DATA = 3

// seek to the next hole
const _SEEK_HOLE = 4

// ENOATTR indicates that an extended attribute was not present.
// const ENOATTR = xattr.ENOATTR

func fromTimeToTimespec(tmsp time.Time) syscall.Timespec {
	return syscall.Timespec{
		Sec:  tmsp.Unix(),
		Nsec: int64(tmsp.Nanosecond()),
	}
}

func fromFileInfoToStat(fileInfo fs.FileInfo, mountConfig MountConfig) syscall.Stat_t {
	var mode uint32
	if fileInfo.IsDir() {
		// Set directory type flag and permissions
		mode = syscall.S_IFDIR | uint32(mountConfig.DirPermission)
	} else {
		// Set regular file type flag and permissions
		mode = syscall.S_IFREG | uint32(mountConfig.FilePermission)
	}

	tmsp := fromTimeToTimespec(fileInfo.ModTime())
	log.Printf("Creating stat for %s: isDir=%v, mode=%o", fileInfo.Name(), fileInfo.IsDir(), mode)
	return syscall.Stat_t{
		Dev:   0,
		Ino:   1,
		Mode:  mode,
		Nlink: 1,
		Uid:   mountConfig.UID,
		Gid:   mountConfig.GID,
		Rdev:  0,
		Size:  fileInfo.Size(),
		Atim:  tmsp,
		Mtim:  tmsp,
		Ctim:  tmsp,
	}
}

// ToErrno exhumes the syscall.Errno error from wrapped error values.
func ToErrno(err error) syscall.Errno {
	s := fuse.ToStatus(err)
	log.Println(err.Error())
	return syscall.Errno(s)
}

// fuse.Stat_t{Dev:0x0, Ino:0x1, Mode:0x41ff, Nlink:0x1, Uid:0x0, Gid:0x0, Rdev:0x0, Size:0, Atim:fuse.Timespec{Sec:1741682324, Nsec:24060231}, Mtim:fuse.Timespec{Sec:1741682333, Nsec:884306899}, Ctim:fuse.Timespec{Sec:1741682333, Nsec:884306899}, Blksize:0, Blocks:0, Birthtim:fuse.Timespec{Sec:1741682324, Nsec:24060231}, Flags:0x0
