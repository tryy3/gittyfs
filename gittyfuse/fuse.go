/*
 * memfs.go
 *
 * Copyright 2017-2022 Bill Zissimopoulos
 */
/*
 * This file is part of Cgofuse.
 *
 * It is licensed under the MIT license. The full license text can be found
 * in the License.txt file at the root of this project.
 */

package gittyfuse

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/go-git/go-billy/v5"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/tryy3/gittyfs/manager"
)

// Ensure that we implement NodeOnAdder
var _ = (fs.NodeOnAdder)((*Filesystem)(nil))

type Filesystem struct {
	*GittyDir
	wt          billy.Filesystem
	manager     *manager.Manager
	mountServer *fuse.Server
	UID         string
	GID         string
}

func traverseTree(ctx context.Context, node *fs.Inode, wt billy.Filesystem, path string, manager *manager.Manager) {
	fmt.Printf("Traversing tree at: %s\n", path)
	files, err := wt.ReadDir(path)
	if err != nil {
		log.Fatalf("read dir: %s", err)
	}

	for _, file := range files {
		if file.IsDir() {
			filePath := filepath.Join(path, file.Name())
			// Create a GittyDir for directories
			dir := NewGittyDir(filePath, wt, manager)
			fmt.Printf("Adding dir: %s\n", file.Name())

			dirNode := node.NewPersistentInode(ctx, dir, fs.StableAttr{Mode: syscall.S_IFDIR})
			node.AddChild(file.Name(), dirNode, true)
			traverseTree(ctx, dirNode, wt, filepath.Join(path, file.Name()), manager)
		} else {
			filePath := filepath.Join(path, file.Name())
			fmt.Printf("Adding file: %s\n", filePath)
			gitFile, err := wt.OpenFile(filePath, os.O_RDONLY, 0)
			if err != nil {
				log.Fatalf("open file: %s", err)
			}
			defer gitFile.Close()

			content, err := io.ReadAll(gitFile)
			if err != nil {
				log.Fatalf("read file: %s", err)
			}

			// create a GittyFile for handling file operations
			gittyFile := NewGittyFileWithContent(filePath, wt, manager, content)

			child := node.NewPersistentInode(ctx, gittyFile, fs.StableAttr{})
			node.AddChild(file.Name(), child, true)
		}
	}

	fmt.Printf("node: %v\n", node)
}

func (self *Filesystem) OnAdd(ctx context.Context) {
	traverseTree(ctx, &self.Inode, self.wt, "", self.manager)
}

func (self *Filesystem) Mount(path string) {
	// Get current user's UID and GID
	var err error
	var uid int
	var gid int
	if self.UID != "" {
		uid, err = strconv.Atoi(self.UID)
		if err != nil {
			log.Fatalf("invalid uid: %s", self.UID)
		}
	} else {
		uid = os.Getuid()
	}
	if self.GID != "" {
		gid, err = strconv.Atoi(self.GID)
		if err != nil {
			log.Fatalf("invalid gid: %s", self.GID)
		}
	} else {
		gid = os.Getgid()
	}

	server, err := fs.Mount(path, self, &fs.Options{
		// Set proper ownership
		UID: uint32(uid),
		GID: uint32(gid),

		MountOptions: fuse.MountOptions{
			AllowOther: true,
			Debug:      true,
		},
	})

	self.mountServer = server
	log.Printf("Mounted on %s", path)
	log.Printf("Unmount by calling 'fusermount -u %s'", path)

	if err != nil {
		log.Panic(err)
	}
}

func (self *Filesystem) Unmount() {
	self.mountServer.Unmount()
}

func NewFilesystem(wt billy.Filesystem, manager *manager.Manager, UID, GID string) *Filesystem {
	dir := NewGittyDir("", wt, manager)

	return &Filesystem{
		GittyDir: dir,
		wt:       wt,
		manager:  manager,
		UID:      UID,
		GID:      GID,
	}
}
