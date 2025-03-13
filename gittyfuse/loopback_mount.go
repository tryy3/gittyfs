package gittyfuse

import (
	"log"
	"os"

	"github.com/go-git/go-billy/v5"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

const (
	DefaultFilePermission = 0644
	DefaultDirPermission  = 0755
)

type MountConfig struct {
	UID            uint32
	GID            uint32
	FilePermission os.FileMode
	DirPermission  os.FileMode
}

func NewMountConfig() MountConfig {
	return MountConfig{
		FilePermission: DefaultFilePermission,
		DirPermission:  DefaultDirPermission,
	}
}

var mountServer *fuse.Server

func MountLoopback(wt billy.Filesystem, path string, config MountConfig) {
	// Get current user's UID and GID
	var err error
	var uid int
	var gid int
	if config.UID != 0 {
		uid = int(config.UID)
	} else {
		uid = os.Getuid()
	}
	if config.GID != 0 {
		gid = int(config.GID)
	} else {
		gid = os.Getgid()
	}

	loopback, err := NewLoopbackRoot(wt, config)
	if err != nil {
		log.Panic(err)
	}

	server, err := fs.Mount(path, loopback, &fs.Options{
		// Set proper ownership
		UID: uint32(uid),
		GID: uint32(gid),

		MountOptions: fuse.MountOptions{
			AllowOther: true,
			Debug:      true,
		},
	})
	mountServer = server

	log.Printf("Mounted on %s", path)
	log.Printf("Unmount by calling 'fusermount -u %s'", path)

	if err != nil {
		log.Panic(err)
	}
}

func UnmountLoopback() {
	mountServer.Unmount()
	mountServer = nil
}
