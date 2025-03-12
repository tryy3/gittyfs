package gittyfuse

import (
	"context"
	"log"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/tryy3/gittyfs/manager"
)

// GittyFile implements a file with better write support
type GittyFile struct {
	fs.Inode
	mu      sync.Mutex
	content []byte
	path    string
	wt      billy.Filesystem
	dirty   bool
	manager *manager.Manager
}

// Ensure it implements the right interfaces
var _ = (fs.NodeOpener)((*GittyFile)(nil))
var _ = (fs.NodeWriter)((*GittyFile)(nil))
var _ = (fs.NodeFsyncer)((*GittyFile)(nil))
var _ = (fs.NodeGetattrer)((*GittyFile)(nil))
var _ = (fs.NodeSetattrer)((*GittyFile)(nil))
var _ = (fs.NodeUnlinker)((*GittyFile)(nil))

func NewGittyFile(path string, wt billy.Filesystem, manager *manager.Manager) *GittyFile {
	return &GittyFile{
		content: []byte{},
		path:    path,
		wt:      wt,
		manager: manager,
	}
}

func NewGittyFileWithContent(path string, wt billy.Filesystem, manager *manager.Manager, content []byte) *GittyFile {
	return &GittyFile{
		content: content,
		path:    path,
		wt:      wt,
		manager: manager,
	}
}

// Unlink handles file deletion
func (f *GittyFile) Unlink(ctx context.Context, name string) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Log the unlink operation
	log.Printf("Unlink called on file: %s with name: %s", f.path, name)

	// Since this is called on the file itself rather than the directory,
	// we typically just need to perform cleanup

	// For regular files, name should be empty or match the base name
	if name != "" {
		// This shouldn't happen for a file, but log it if it does
		log.Printf("Warning: Unlink on file %s with non-empty name %s", f.path, name)
	}

	// If the file is dirty, we may want to clear its content
	if f.dirty {
		f.content = nil
		f.dirty = false
	}

	// Notify the manager about the deletion if needed
	if f.manager != nil {
		f.manager.NotifyChange(f.path, "delete")
	}

	return 0
}

// Open handles file opening
func (f *GittyFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	return f, 0, 0
}

// Write implements writing to the file
func (f *GittyFile) Write(ctx context.Context, fh fs.FileHandle, data []byte, off int64) (uint32, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Expand the content slice if needed
	if int64(len(f.content)) < off+int64(len(data)) {
		newSlice := make([]byte, off+int64(len(data)))
		copy(newSlice, f.content)
		f.content = newSlice
	}

	n := copy(f.content[off:], data)
	f.dirty = true
	return uint32(n), 0
}

// Read implements reading from the file
func (f *GittyFile) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if off >= int64(len(f.content)) {
		return fuse.ReadResultData([]byte{}), 0
	}

	end := off + int64(len(dest))
	if end > int64(len(f.content)) {
		end = int64(len(f.content))
	}

	return fuse.ReadResultData(f.content[off:end]), 0
}

// Fsync persists the file to the underlying filesystem
func (f *GittyFile) Fsync(ctx context.Context, fh fs.FileHandle, flags uint32) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.dirty {
		return 0
	}

	// Write back to the billy filesystem
	file, err := f.wt.OpenFile(f.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		log.Printf("Error opening file for fsync: %v", err)
		return syscall.EIO
	}
	defer file.Close()

	_, err = file.Write(f.content)
	if err != nil {
		log.Printf("Error writing file during fsync: %v", err)
		return syscall.EIO
	}

	f.dirty = false
	f.manager.NotifyChange(f.path, "write")
	return 0
}

// Getattr returns file attributes
func (f *GittyFile) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()

	// First try to get attributes from the actual filesystem
	info, err := f.wt.Stat(f.path)
	if err == nil {
		// File exists in the filesystem
		out.Size = uint64(info.Size())
		out.Mode = uint32(info.Mode())

		// Set times based on ModTime (billy doesn't expose other times)
		mtime := info.ModTime()
		out.Atime = uint64(mtime.Unix())
		out.Atimensec = uint32(mtime.Nanosecond())
		out.Mtime = uint64(mtime.Unix())
		out.Mtimensec = uint32(mtime.Nanosecond())
		out.Ctime = uint64(mtime.Unix())
		out.Ctimensec = uint32(mtime.Nanosecond())

		// If we have in-memory content that's different (dirty), use its size
		if f.dirty {
			out.Size = uint64(len(f.content))
		}

		return 0
	}

	// Fall back to in-memory state if file doesn't exist yet in filesystem
	// (this could happen with newly created files before they're synced)
	out.Size = uint64(len(f.content))
	out.Mode = 0666 // Default mode for files

	// Use current time as fallback
	t := time.Now()
	out.SetTimes(&t, &t, &t)

	return 0
}

// Setattr handles attribute changes for a file
func (f *GittyFile) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	f.mu.Lock()
	defer f.mu.Unlock()

	log.Printf("Setattr on file: %s", f.path)

	// Check what attributes are being set
	valid := in.Valid

	// Handle size change (truncate)
	if valid&fuse.FATTR_SIZE != 0 {
		newSize := in.Size

		// Resize the content slice
		if newSize == 0 {
			// Truncate to empty
			f.content = []byte{}
		} else if newSize < uint64(len(f.content)) {
			// Truncate to smaller size
			f.content = f.content[:newSize]
		} else if newSize > uint64(len(f.content)) {
			// Expand the file with zeros
			newContent := make([]byte, newSize)
			copy(newContent, f.content)
			f.content = newContent
		}

		f.dirty = true
		log.Printf("File %s size changed to %d", f.path, newSize)
	}

	// Handle mode change
	if valid&fuse.FATTR_MODE != 0 {
		// Billy doesn't support chmod directly, but we log it
		log.Printf("Mode change requested to %o for %s", in.Mode, f.path)
	}

	// Handle ownership changes
	if valid&fuse.FATTR_UID != 0 || valid&fuse.FATTR_GID != 0 {
		// Billy doesn't support chown directly, but we log it
		log.Printf("Ownership change requested for %s", f.path)
	}

	// Handle time changes
	if valid&(fuse.FATTR_ATIME|fuse.FATTR_MTIME|fuse.FATTR_CTIME) != 0 {
		log.Printf("Time change requested for %s", f.path)
		// If your implementation of billy supports this, you could implement it
	}

	// Fill out the output attributes
	out.Size = uint64(len(f.content))
	out.Mode = 0666 // Use the mode you want for files

	// Set times
	t := time.Now()

	// Use input times if provided
	if valid&fuse.FATTR_ATIME != 0 {
		out.Atime = in.Atime
		out.Atimensec = in.Atimensec
	} else {
		out.Atime = uint64(t.Unix())
		out.Atimensec = uint32(t.Nanosecond())
	}

	if valid&fuse.FATTR_MTIME != 0 {
		out.Mtime = in.Mtime
		out.Mtimensec = in.Mtimensec
	} else {
		out.Mtime = uint64(t.Unix())
		out.Mtimensec = uint32(t.Nanosecond())
	}

	return 0
}
