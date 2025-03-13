package gittyfuse

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-git/go-billy/v5"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/tryy3/gittyfs/manager"
)

// GittyDir implements directory operations
type GittyDir struct {
	fs.Inode
	path    string
	wt      billy.Filesystem
	manager *manager.Manager
}

// Implement NodeCreater to allow creating new files
var _ = (fs.NodeCreater)((*GittyDir)(nil))
var _ = (fs.NodeSetattrer)((*GittyDir)(nil))
var _ = (fs.NodeUnlinker)((*GittyDir)(nil))
var _ = (fs.NodeGetattrer)((*GittyDir)(nil))
var _ = (fs.NodeReaddirer)((*GittyDir)(nil))
var _ = (fs.NodeRenamer)((*GittyDir)(nil))
var _ = (fs.NodeMkdirer)((*GittyDir)(nil))

func NewGittyDir(path string, wt billy.Filesystem, manager *manager.Manager) *GittyDir {
	return &GittyDir{
		path:    path,
		wt:      wt,
		manager: manager,
	}
}

// Readdir is part of the NodeReaddirer interface
func (d *GittyDir) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	r := make([]fuse.DirEntry, 0, 0)
	log.Println("Readdir hit")
	log.Printf("Children: %#v\n", d.Children())
	return fs.NewListDirStream(r), 0
}

// Create creates a new file in the directory
func (d *GittyDir) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	log.Printf("Create: %s", name)
	path := filepath.Join(d.path, name)

	// Create an empty GittyFile
	gfile := NewGittyFile(path, d.wt, d.manager)

	// Create the file in the billy filesystem
	file, err := d.wt.Create(path)
	if err != nil {
		log.Printf("Error creating file: %v", err)
		return nil, nil, 0, syscall.EIO
	}
	defer file.Close()

	// Create the child node
	child := d.NewPersistentInode(ctx, gfile, fs.StableAttr{})
	d.AddChild(name, child, true)

	// Setup entry attributes
	out.Mode = uint32(0666)
	t := time.Now()
	out.SetTimes(&t, &t, &t)
	log.Printf("Created file: %s\n", path)
	d.manager.NotifyChange(path, "create")

	return child, gfile, 0, 0
}

// Getattr returns file attributes
func (d *GittyDir) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	// Get information from the underlying filesystem
	info, err := d.wt.Stat(d.path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Directory %s not found in Getattr", d.path)
			return syscall.ENOENT
		}
		log.Printf("Error getting directory info in Getattr: %v", err)
		return syscall.EIO
	}

	// Set mode to directory with appropriate permissions
	if info.IsDir() {
		out.Mode = uint32(info.Mode() | syscall.S_IFDIR)
	} else {
		// This should not happen, but if it does, force directory mode
		out.Mode = uint32(info.Mode() | syscall.S_IFDIR)
		log.Printf("Warning: Path %s is not a directory in Getattr", d.path)
	}

	// Size for directories is typically 4096 bytes on many filesystems
	// but we'll use the actual size from the info
	out.Size = uint64(info.Size())

	// Set times based on ModTime (billy doesn't expose other times)
	mtime := info.ModTime()
	out.Atime = uint64(mtime.Unix())
	out.Atimensec = uint32(mtime.Nanosecond())
	out.Mtime = uint64(mtime.Unix())
	out.Mtimensec = uint32(mtime.Nanosecond())
	out.Ctime = uint64(mtime.Unix())
	out.Ctimensec = uint32(mtime.Nanosecond())

	// On most Unix filesystems, the link count for a directory
	// is at least 2 (for . and ..) plus the number of subdirectories
	// but we'll set it to 2 as a safe minimum
	out.Nlink = 2

	return 0
}

func (d *GittyDir) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	log.Printf("Setattr on directory: %s", d.path)

	// Check what attributes are being set
	valid := in.Valid

	// Get file info from the underlying filesystem
	info, err := d.wt.Stat(d.path)
	if err != nil {
		log.Printf("Error getting file info in Setattr: %v", err)
		return syscall.EIO
	}

	// Handle mode change
	if valid&fuse.FATTR_MODE != 0 {
		// Billy doesn't support chmod directly, but we can log it
		log.Printf("Mode change requested to %o for %s", in.Mode, d.path)
		// Some implementations of billy might support this through special interfaces
	}

	// Handle ownership changes
	if valid&fuse.FATTR_UID != 0 || valid&fuse.FATTR_GID != 0 {
		// Billy doesn't support chown directly, but we can log it
		log.Printf("Ownership change requested for %s", d.path)
		// Some implementations of billy might support this through special interfaces
	}

	// Handle size change (truncate) - not applicable for directories
	if valid&fuse.FATTR_SIZE != 0 {
		log.Printf("Size change requested for directory %s (ignored)", d.path)
	}

	// Handle time changes
	// Billy doesn't provide direct ways to set these, but we should acknowledge them
	if valid&(fuse.FATTR_ATIME|fuse.FATTR_MTIME|fuse.FATTR_CTIME) != 0 {
		log.Printf("Time change requested for %s", d.path)
		// If your implementation of billy supports this, you might implement it here
	}

	// Fill out attributes
	out.Mode = uint32(info.Mode())
	out.Size = uint64(info.Size())

	// Set times based on info or the input
	if valid&fuse.FATTR_ATIME != 0 {
		out.Atime = in.Atime
		out.Atimensec = in.Atimensec
	} else {
		// Use the existing time
		atime := info.ModTime() // Billy doesn't expose access time separately
		out.Atime = uint64(atime.Unix())
		out.Atimensec = uint32(atime.Nanosecond())
	}

	if valid&fuse.FATTR_MTIME != 0 {
		out.Mtime = in.Mtime
		out.Mtimensec = in.Mtimensec
	} else {
		// Use the existing time
		mtime := info.ModTime()
		out.Mtime = uint64(mtime.Unix())
		out.Mtimensec = uint32(mtime.Nanosecond())
	}

	return 0
}

// Unlink removes a file from the directory
func (d *GittyDir) Unlink(ctx context.Context, name string) syscall.Errno {
	path := filepath.Join(d.path, name)
	log.Printf("Unlink: %s", path)

	// Remove from the billy filesystem
	err := d.wt.Remove(path)
	if err != nil {
		log.Printf("Error removing file %s: %v", path, err)
		if os.IsNotExist(err) {
			return syscall.ENOENT
		}
		return syscall.EIO
	}

	// Remove the child from the inode
	d.Inode.RmChild(name)

	// Notify manager about the deletion
	if d.manager != nil {
		d.manager.NotifyChange(path, "delete")
	}

	return 0
}

// Rmdir removes an empty directory
func (d *GittyDir) Rmdir(ctx context.Context, name string) syscall.Errno {
	path := filepath.Join(d.path, name)
	log.Printf("Rmdir: %s", path)

	// Check if directory exists and is empty
	entries, err := d.wt.ReadDir(path)
	if err != nil {
		log.Printf("Error reading directory %s: %v", path, err)
		if os.IsNotExist(err) {
			return syscall.ENOENT
		}
		return syscall.EIO
	}

	// POSIX requires directories to be empty before removal
	if len(entries) > 0 {
		log.Printf("Cannot remove non-empty directory: %s", path)
		return syscall.ENOTEMPTY
	}

	// Remove the directory from the billy filesystem
	err = d.wt.Remove(path)
	if err != nil {
		log.Printf("Error removing directory %s: %v", path, err)
		return syscall.EIO
	}

	// Remove the child from the inode
	d.Inode.RmChild(name)

	// Notify manager about the deletion
	if d.manager != nil {
		d.manager.NotifyChange(path, "rmdir")
	}

	return 0
}

func (d *GittyDir) findFile(p string) *fs.Inode {
	// First traverse up to the parent directory
	var traverseUp func(node *fs.Inode) *fs.Inode
	traverseUp = func(node *fs.Inode) *fs.Inode {
		if node.IsRoot() {
			return node
		}
		f, parent := node.Parent()
		log.Printf("Test: %#v\n", f)
		return traverseUp(parent)
	}
	parent := traverseUp(&d.Inode)
	log.Printf("parent: %#v\n", parent)

	// Get filename from path
	pathSplit := strings.Split(p, string(os.PathSeparator))
	for _, v := range pathSplit {
		// look in directory
		children := parent.Children()
		for name, child := range children {
			log.Printf("Name: %#v\n", name)
			if name == v {
				log.Printf("Found: %#v\n", name)
				parent = child
				break
			}
		}
	}

	return parent
}

// Rename implements the NodeRenamer interface for GittyDir
func (d *GittyDir) Rename(ctx context.Context, oldName string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	log.Printf("Rename directory entry: %s -> %s", oldName, newName)
	log.Printf("Self: %#v\n", spew.Sdump(d))

	// Old path, new path
	oldPath := filepath.Join(d.path, oldName)
	log.Printf("oldPath: '%s'\n", oldPath)

	var newParentPath string
	var newPath string

	if dir, ok := newParent.(*GittyDir); ok {
		log.Printf("dir: %#v\n", dir)
		log.Println("Directory")
		newParentPath = dir.path
		newPath = filepath.Join(newParentPath, newName)
	} else if file, ok := newParent.(*GittyFile); ok {
		log.Printf("file: %#v\n", file)
		log.Println("File")
	} else if root, ok := newParent.(*Filesystem); ok {
		log.Printf("root: %#v\n", root)
		log.Println("Root")
		newParentPath = root.path
		newPath = filepath.Join(newParentPath, newName)
		newParent = &root.Inode
	} else {
		log.Println("Unknown")
		newParentPath = ""
		newPath = newName
	}
	log.Printf("newParent: %#v\n", spew.Sdump(newParent))

	log.Printf("newParentPath: %s\n", newParentPath)
	err := d.wt.Rename(oldPath, newPath)
	if err != nil {
		log.Printf("Error renaming directory %s: %v", oldPath, err)
		return syscall.EIO
	}

	log.Printf("NewPath: '%s'\n", newPath)
	// Retrieve current file
	oldFile := d.findFile(oldPath)
	newParentDir := d.findFile(newParentPath)
	log.Printf("OldFile: %#v\n", spew.Sdump(oldFile))
	log.Printf("NewFile: %#v\n", spew.Sdump(newParentDir))

	//Forget the old file
	_, oldParentDir := oldFile.Parent()
	// _, newParentDir := newFile.Parent()
	// oldParentDir.ExchangeChild(oldName, newParent.EmbeddedInode(), newName)
	// ok := oldParentDir.MvChild(oldName, newParentDir, newName, true)
	// if !ok {
	// 	log.Printf("Error moving child %s", oldName)
	// 	return syscall.EIO
	// }
	// child, ok := oldParentDir.Children()[newName]
	// if !ok {
	// 	log.Printf("Error finding child %s", newName)
	// 	return syscall.EIO
	// }
	// child.Operations().(*GittyFile).path = newPath
	ok, _ := oldParentDir.RmChild(oldName)
	if !ok {
		log.Printf("Error removing child %s", oldName)
		return syscall.EIO
	}

	// Add the new file
	var embed = oldFile.Operations().(*GittyFile)
	// ok = newParentDir.AddChild(newName, oldFile, true)
	// gittyFile := NewGittyFileWithContent(newPath, embed.wt, embed.manager, embed.content)

	child := newParentDir.NewPersistentInode(ctx, embed, fs.StableAttr{Gen: 532425})
	ok = newParentDir.AddChild(newName, child, true)
	if !ok {
		log.Printf("Error adding child %s", newName)
		return syscall.EIO
	}

	// log.Printf("Children after: %#v\n", oldParentDir.Children())
	// log.Printf("Parent after: %#v\n", newParent.EmbeddedInode().Children())

	// log.Printf("OldParentDir: %#v\n", oldParentDir)
	// log.Printf("NewParent: %#v\n", newParent.EmbeddedInode())

	// Notify manager about deleting oldPath and creating newPath

	log.Printf("OldParentDir: %s\n", spew.Sdump(oldParentDir))
	log.Printf("NewParent: %s\n", spew.Sdump(newParentDir))

	d.manager.NotifyChange(oldPath, "delete")
	d.manager.NotifyChange(newPath, "create")

	return 0
}

// Mkdir implements the NodeMkdirer interface for creating directories
func (d *GittyDir) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	path := filepath.Join(d.path, name)
	log.Printf("Mkdir: %s with mode %o", path, mode)

	// Create the directory in the underlying filesystem
	err := d.wt.MkdirAll(path, os.FileMode(mode))
	if err != nil {
		log.Printf("Error creating directory %s: %v", path, err)
		return nil, syscall.EIO
	}

	// Create a new GittyDir for the new directory
	newDir := NewGittyDir(path, d.wt, d.manager)

	// Create a persistent inode for the new directory
	child := d.NewPersistentInode(ctx, newDir, fs.StableAttr{Mode: syscall.S_IFDIR})

	// Add the new directory as a child of this directory
	d.AddChild(name, child, true)

	// Set attributes for the new directory
	out.Mode = mode | syscall.S_IFDIR

	// Set times for the new directory
	t := time.Now()
	out.SetTimes(&t, &t, &t)

	// Default directory size is typically 4096 bytes
	out.Size = 4096

	// Set link count to 2 (. and ..)
	out.Nlink = 2

	// Notify manager about the new directory
	if d.manager != nil {
		d.manager.NotifyChange(path, "mkdir")
	}

	log.Printf("Successfully created directory: %s", path)
	return child, 0
}
