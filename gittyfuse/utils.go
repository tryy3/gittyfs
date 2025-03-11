package gittyfuse

import (
	"fmt"
	"io/fs"
	"time"

	"github.com/winfsp/cgofuse/fuse"
)

func fromTimeToTimespec(tmsp time.Time) fuse.Timespec {
	return fuse.Timespec{
		Sec:  tmsp.Unix(),
		Nsec: int64(tmsp.Nanosecond()),
	}
}

func fromFileInfoToStat(fileInfo fs.FileInfo) fuse.Stat_t {
	fmt.Printf("%#v\n", fileInfo)
	uid, gid, _ := fuse.Getcontext()
	tmsp := fromTimeToTimespec(fileInfo.ModTime())
	return fuse.Stat_t{
		Dev:      0,
		Ino:      1,
		Mode:     uint32(fileInfo.Mode()),
		Nlink:    1,
		Uid:      uid,
		Gid:      gid,
		Rdev:     0,
		Size:     fileInfo.Size(),
		Atim:     tmsp,
		Mtim:     tmsp,
		Ctim:     tmsp,
		Birthtim: tmsp,
		Flags:    0,
	}
}

// fuse.Stat_t{Dev:0x0, Ino:0x1, Mode:0x41ff, Nlink:0x1, Uid:0x0, Gid:0x0, Rdev:0x0, Size:0, Atim:fuse.Timespec{Sec:1741682324, Nsec:24060231}, Mtim:fuse.Timespec{Sec:1741682333, Nsec:884306899}, Ctim:fuse.Timespec{Sec:1741682333, Nsec:884306899}, Blksize:0, Blocks:0, Birthtim:fuse.Timespec{Sec:1741682324, Nsec:24060231}, Flags:0x0
