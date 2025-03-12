package gittyfuse

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"

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

func NewGittyDir(path string, wt billy.Filesystem, manager *manager.Manager) *GittyDir {
	return &GittyDir{
		path:    path,
		wt:      wt,
		manager: manager,
	}
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
