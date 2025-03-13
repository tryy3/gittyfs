// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gittyfuse

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"

	"github.com/go-git/go-billy/v5"
	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"golang.org/x/sys/unix"
)

// LoopbackRoot holds the parameters for creating a new loopback
// filesystem. Loopback filesystem delegate their operations to an
// underlying POSIX file system.
type LoopbackRoot struct {
	// The path to the root of the underlying file system.
	wt          billy.Filesystem
	mountConfig MountConfig
	rootPath    string

	// The device on which the Path resides. This must be set if
	// the underlying filesystem crosses file systems.
	Dev uint64

	// NewNode returns a new InodeEmbedder to be used to respond
	// to a LOOKUP/CREATE/MKDIR/MKNOD opcode. If not set, use a
	// LoopbackNode.
	NewNode func(rootData *LoopbackRoot, parent *fs.Inode, name string, st *syscall.Stat_t) fs.InodeEmbedder

	// RootNode is the root of the Loopback. This must be set if
	// the Loopback file system is not the root of the FUSE
	// mount. It is set automatically by NewLoopbackRoot.
	RootNode fs.InodeEmbedder
}

func (r *LoopbackRoot) newNode(parent *fs.Inode, name string, st *syscall.Stat_t) fs.InodeEmbedder {
	if r.NewNode != nil {
		return r.NewNode(r, parent, name, st)
	}
	return &LoopbackNode{
		RootData: r,
	}
}

func (r *LoopbackRoot) idFromStat(st *syscall.Stat_t) fs.StableAttr {
	// We compose an inode number by the underlying inode, and
	// mixing in the device number. In traditional filesystems,
	// the inode numbers are small. The device numbers are also
	// small (typically 16 bit). Finally, we mask out the root
	// device number of the root, so a loopback FS that does not
	// encompass multiple mounts will reflect the inode numbers of
	// the underlying filesystem
	swapped := (uint64(st.Dev) << 32) | (uint64(st.Dev) >> 32)
	swappedRootDev := (r.Dev << 32) | (r.Dev >> 32)
	return fs.StableAttr{
		Mode: uint32(st.Mode),
		Gen:  1,
		// This should work well for traditional backing FSes,
		// not so much for other go-fuse FS-es
		Ino: (swapped ^ swappedRootDev) ^ st.Ino,
	}
}

// LoopbackNode is a filesystem node in a loopback file system. It is
// public so it can be used as a basis for other loopback based
// filesystems. See NewLoopbackFile or LoopbackRoot for more
// information.
type LoopbackNode struct {
	fs.Inode

	// RootData points back to the root of the loopback filesystem.
	RootData *LoopbackRoot
}

// loopbackNodeEmbedder can only be implemented by the LoopbackNode
// concrete type.
type loopbackNodeEmbedder interface {
	loopbackNode() *LoopbackNode
}

func (n *LoopbackNode) loopbackNode() *LoopbackNode {
	return n
}

// var _ = (fs.NodeStatfser)((*LoopbackNode)(nil))

// func (n *LoopbackNode) Statfs(ctx context.Context, out *fuse.StatfsOut) syscall.Errno {
// 	wt := n.RootData.wt
// 	stat, err := wt.Stat(n.path())
// 	if err != nil {
// 		return ToErrno(err)
// 	}
// 	syscallStat := fromFileInfoToStat(stat, n.RootData.mountConfig)
// 	out.FromStatfsT(&syscallStat)
// 	return OK
// }

// path returns the full path to the file in the underlying file
// system.
func (n *LoopbackNode) root() *fs.Inode {
	var rootNode *fs.Inode
	if n.RootData.RootNode != nil {
		rootNode = n.RootData.RootNode.EmbeddedInode()
	} else {
		rootNode = n.Root()
	}

	return rootNode
}

func (n *LoopbackNode) path() string {
	path := n.Path(n.root())
	return filepath.Join(n.RootData.rootPath, path)
}

var _ = (fs.NodeLookuper)((*LoopbackNode)(nil))

func (n *LoopbackNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	p := filepath.Join(n.path(), name)

	st, err := n.RootData.wt.Stat(p)
	if err != nil {
		return nil, ToErrno(fmt.Errorf("failed to stat %s: %w", p, err))
	}

	syscallStat := fromFileInfoToStat(st, n.RootData.mountConfig)

	out.Attr.FromStat(&syscallStat)
	node := n.RootData.newNode(n.EmbeddedInode(), name, &syscallStat)
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&syscallStat))
	return ch, 0
}

// preserveOwner sets uid and gid of `path` according to the caller information
// in `ctx`.
func (n *LoopbackNode) preserveOwner(ctx context.Context, path string) error {
	// if os.Getuid() != 0 {
	// 	return nil
	// }
	// caller, ok := fuse.FromContext(ctx)
	// if !ok {
	// 	return nil
	// }

	// n.RootData.wt.Chown(path, int(caller.Uid), int(caller.Gid))
	return nil
}

// var _ = (fs.NodeMknoder)((*LoopbackNode)(nil))

// func (n *LoopbackNode) Mknod(ctx context.Context, name string, mode, rdev uint32, out *fuse.EntryOut) (*Inode, syscall.Errno) {
// 	p := filepath.Join(n.path(), name)
// 	err := syscall.Mknod(p, mode, intDev(rdev))
// 	if err != nil {
// 		return nil, ToErrno(err)
// 	}
// 	n.preserveOwner(ctx, p)
// 	st := syscall.Stat_t{}
// 	if err := syscall.Lstat(p, &st); err != nil {
// 		syscall.Rmdir(p)
// 		return nil, ToErrno(err)
// 	}

// 	out.Attr.FromStat(&st)

// 	node := n.RootData.newNode(n.EmbeddedInode(), name, &st)
// 	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st))

// 	return ch, 0
// }

var _ = (fs.NodeMkdirer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	path := filepath.Join(n.path(), name)
	err := n.RootData.wt.MkdirAll(path, os.FileMode(mode))
	if err != nil {
		return nil, ToErrno(fmt.Errorf("failed to mkdir %s: %w", path, err))
	}
	n.preserveOwner(ctx, path)
	fileInfo, err := n.RootData.wt.Lstat(path)
	if err != nil {
		n.RootData.wt.Remove(path)
		return nil, ToErrno(fmt.Errorf("failed to lstat %s: %w", path, err))
	}
	syscallStat := fromFileInfoToStat(fileInfo, n.RootData.mountConfig)

	out.Attr.FromStat(&syscallStat)

	node := n.RootData.newNode(n.EmbeddedInode(), name, &syscallStat)
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&syscallStat))

	return ch, 0
}

var _ = (fs.NodeRmdirer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Rmdir(ctx context.Context, name string) syscall.Errno {
	p := filepath.Join(n.path(), name)
	err := n.RootData.wt.Remove(p)
	if err != nil {
		return ToErrno(fmt.Errorf("failed to rmdir %s: %w", p, err))
	}
	return 0
}

var _ = (fs.NodeUnlinker)((*LoopbackNode)(nil))

func (n *LoopbackNode) Unlink(ctx context.Context, name string) syscall.Errno {
	p := filepath.Join(n.path(), name)
	err := n.RootData.wt.Remove(p)
	if err != nil {
		return ToErrno(fmt.Errorf("failed to unlink %s: %w", p, err))
	}
	return 0
}

var _ = (fs.NodeRenamer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	e2, ok := newParent.(loopbackNodeEmbedder)
	if !ok {
		return syscall.EXDEV
	}

	if e2.loopbackNode().RootData != n.RootData {
		return syscall.EXDEV
	}

	// if flags&RENAME_EXCHANGE != 0 {
	// 	return n.renameExchange(name, e2.loopbackNode(), newName)
	// }

	p1 := filepath.Join(n.path(), name)
	p2 := filepath.Join(e2.loopbackNode().path(), newName)
	err := n.RootData.wt.Rename(p1, p2)
	if err != nil {
		return ToErrno(fmt.Errorf("failed to rename %s to %s: %w", p1, p2, err))
	}
	return 0
}

// func (n *LoopbackNode) renameExchange(name string, newParent *LoopbackNode, newName string) syscall.Errno {
// 	fd1, err := syscall.Open(n.path(), syscall.O_DIRECTORY, 0)
// 	if err != nil {
// 		return ToErrno(err)
// 	}
// 	defer syscall.Close(fd1)
// 	p2 := newParent.path()
// 	fd2, err := syscall.Open(p2, syscall.O_DIRECTORY, 0)
// 	defer syscall.Close(fd2)
// 	if err != nil {
// 		return ToErrno(err)
// 	}

// 	var st syscall.Stat_t
// 	if err := syscall.Fstat(fd1, &st); err != nil {
// 		return ToErrno(err)
// 	}

// 	// Double check that nodes didn't change from under us.
// 	if n.root() != n.EmbeddedInode() && n.Inode.StableAttr().Ino != n.RootData.idFromStat(&st).Ino {
// 		return syscall.EBUSY
// 	}
// 	if err := syscall.Fstat(fd2, &st); err != nil {
// 		return ToErrno(err)
// 	}

// 	if (newParent.root() != newParent.EmbeddedInode()) && newParent.Inode.StableAttr().Ino != n.RootData.idFromStat(&st).Ino {
// 		return syscall.EBUSY
// 	}

// 	return ToErrno(renameat.Renameat(fd1, name, fd2, newName, renameat.RENAME_EXCHANGE))
// }

var _ = (fs.NodeCreater)((*LoopbackNode)(nil))

func (n *LoopbackNode) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (inode *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	path := filepath.Join(n.path(), name)
	flags = flags &^ syscall.O_APPEND

	_, err := n.RootData.wt.Create(path)
	if err != nil {
		return nil, nil, 0, ToErrno(err)
	}
	n.preserveOwner(ctx, path)
	fileInfo, err := n.RootData.wt.Lstat(path)
	if err != nil {
		n.RootData.wt.Remove(path)
		return nil, nil, 0, ToErrno(fmt.Errorf("failed to lstat %s: %w", path, err))
	}
	syscallStat := fromFileInfoToStat(fileInfo, n.RootData.mountConfig)

	node := n.RootData.newNode(n.EmbeddedInode(), name, &syscallStat)
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&syscallStat))
	lf := NewLoopbackFile(n.RootData.wt, path)

	out.FromStat(&syscallStat)
	return ch, lf, 0, 0
}

var _ = (fs.NodeSymlinker)((*LoopbackNode)(nil))

func (n *LoopbackNode) Symlink(ctx context.Context, target, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	path := filepath.Join(n.path(), name)
	err := n.RootData.wt.Symlink(target, path)
	if err != nil {
		return nil, ToErrno(err)
	}

	n.preserveOwner(ctx, path)
	fileInfo, err := n.RootData.wt.Lstat(path)
	if err != nil {
		n.RootData.wt.Remove(path)
		return nil, ToErrno(fmt.Errorf("failed to lstat %s: %w", path, err))
	}
	syscallStat := fromFileInfoToStat(fileInfo, n.RootData.mountConfig)

	node := n.RootData.newNode(n.EmbeddedInode(), name, &syscallStat)
	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&syscallStat))

	out.Attr.FromStat(&syscallStat)
	return ch, 0
}

// var _ = (fs.NodeLinker)((*LoopbackNode)(nil))

// func (n *LoopbackNode) Link(ctx context.Context, target fs.InodeEmbedder, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
// 	p := filepath.Join(n.path(), name)
// 	err := syscall.Link(filepath.Join(n.RootData.Path, target.EmbeddedInode().Path(nil)), p)
// 	if err != nil {
// 		return nil, ToErrno(err)
// 	}
// 	st := syscall.Stat_t{}
// 	if err := syscall.Lstat(p, &st); err != nil {
// 		syscall.Unlink(p)
// 		return nil, ToErrno(err)
// 	}
// 	node := n.RootData.newNode(n.EmbeddedInode(), name, &st)
// 	ch := n.NewInode(ctx, node, n.RootData.idFromStat(&st))

// 	out.Attr.FromStat(&st)
// 	return ch, 0
// }

var _ = (fs.NodeReadlinker)((*LoopbackNode)(nil))

func (n *LoopbackNode) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	str, err := n.RootData.wt.Readlink(n.path())
	if err != nil {
		return nil, ToErrno(fmt.Errorf("failed to readlink %s: %w", n.path(), err))
	}
	return []byte(str), 0
}

var _ = (fs.NodeOpener)((*LoopbackNode)(nil))

func (n *LoopbackNode) Open(ctx context.Context, flags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	flags = flags &^ syscall.O_APPEND
	p := n.path()
	_, err := n.RootData.wt.Open(p)
	if err != nil {
		return nil, 0, ToErrno(fmt.Errorf("failed to open %s: %w", p, err))
	}
	lf := NewLoopbackFile(n.RootData.wt, p)
	return lf, 0, 0
}

// var _ = (fs.NodeOpendirHandler)((*LoopbackNode)(nil))

// func (n *LoopbackNode) OpendirHandle(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
// 	ds, errno := n.Readdir(ctx)
// 	if errno != 0 {
// 		return nil, 0, errno
// 	}
// 	return ds, 0, errno
// }

var _ = (fs.NodeReaddirer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	path := n.path()
	log.Printf("Readdir starting for: %s", path)

	files, err := n.RootData.wt.ReadDir(path)
	if err != nil {
		log.Printf("Readdir error for %s: %v", path, err)
		return nil, ToErrno(fmt.Errorf("failed to readdir %s: %w", path, err))
	}

	log.Printf("Readdir for %s returned %d entries", path, len(files))

	// Add protection against very large directories
	if len(files) > 1000 {
		log.Printf("WARNING: Large directory at %s with %d entries", path, len(files))
	}

	// Debug: print first few entries
	if len(files) > 0 {
		count := len(files)
		if count > 5 {
			count = 5
		}
		for i := 0; i < count; i++ {
			log.Printf("Entry %d: %s (dir: %v)", i, files[i].Name(), files[i].IsDir())
		}
	}

	dirStream := NewSimpleDirStreamFromFiles(files)
	log.Printf("Created DirStream for %s with %d entries", path, len(files))
	return dirStream, 0
}

var _ = (fs.NodeGetattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	if f != nil {
		if fga, ok := f.(fs.FileGetattrer); ok {
			return fga.Getattr(ctx, out)
		}
	}

	path := n.path()

	var err error
	var fileInfo os.FileInfo
	if &n.Inode == n.Root() {
		fileInfo, err = n.RootData.wt.Stat(path)
	} else {
		fileInfo, err = n.RootData.wt.Lstat(path)
	}

	if err != nil {
		return ToErrno(fmt.Errorf("failed to getattr %s: %w", path, err))
	}

	st := fromFileInfoToStat(fileInfo, n.RootData.mountConfig)
	log.Printf("Getattr for %s: isDir=%v, mode=%o", path, fileInfo.IsDir(), st.Mode)
	out.FromStat(&st)
	return OK
}

var _ = (fs.NodeSetattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Setattr(ctx context.Context, f fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	path := n.path()
	fsa, ok := f.(fs.FileSetattrer)
	if ok && fsa != nil {
		fsa.Setattr(ctx, in, out)
	} else {
		// if m, ok := in.GetMode(); ok {
		// 	if err := syscall.Chmod(p, m); err != nil {
		// 		return ToErrno(err)
		// 	}
		// }

		// uid, uok := in.GetUID()
		// gid, gok := in.GetGID()
		// if uok || gok {
		// 	suid := -1
		// 	sgid := -1
		// 	if uok {
		// 		suid = int(uid)
		// 	}
		// 	if gok {
		// 		sgid = int(gid)
		// 	}
		// 	if err := n.RootData.wt.Chown(path, suid, sgid); err != nil {
		// 		return ToErrno(err)
		// 	}
		// }

		mtime, mok := in.GetMTime()
		atime, aok := in.GetATime()

		if mok || aok {
			ta := unix.Timespec{Nsec: unix.UTIME_OMIT}
			tm := unix.Timespec{Nsec: unix.UTIME_OMIT}
			var err error
			if aok {
				ta, err = unix.TimeToTimespec(atime)
				if err != nil {
					return ToErrno(err)
				}
			}
			if mok {
				tm, err = unix.TimeToTimespec(mtime)
				if err != nil {
					return ToErrno(err)
				}
			}
			ts := []unix.Timespec{ta, tm}
			if err := unix.UtimesNanoAt(unix.AT_FDCWD, path, ts, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				return ToErrno(err)
			}
		}

		// if sz, ok := in.GetSize(); ok {
		// 	if err := syscall.Truncate(p, int64(sz)); err != nil {
		// 		return ToErrno(err)
		// 	}
		// }
	}

	fga, ok := f.(fs.FileGetattrer)
	if ok && fga != nil {
		fga.Getattr(ctx, out)
	} else {
		fileInfo, err := n.RootData.wt.Lstat(path)
		if err != nil {
			return ToErrno(fmt.Errorf("failed to lstat %s: %w", path, err))
		}
		st := fromFileInfoToStat(fileInfo, n.RootData.mountConfig)
		// Don't force directory mode on all files
		// st.Mode = uint32(fileInfo.Mode()) | syscall.S_IFDIR
		log.Printf("Setattr Mode: %o for %s, isDir: %v", st.Mode, path, fileInfo.IsDir())
		out.FromStat(&st)
	}
	return OK
}

var _ = (fs.NodeGetxattrer)((*LoopbackNode)(nil))

func (n *LoopbackNode) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	// Billy filesystem doesn't natively support extended attributes
	// We'll handle some common attributes and return ENODATA for others
	log.Printf("Getxattr requested attr=%s for path=%s", attr, n.path())

	path := n.path()

	// Handle user namespace attributes - we don't support these
	if len(attr) >= 5 && attr[:5] == "user." {
		return 0, syscall.ENODATA
	}

	// Handle system namespace attributes
	if len(attr) >= 7 && attr[:7] == "system." {
		// Handle common system attributes
		if attr == "system.posix_acl_access" || attr == "system.posix_acl_default" {
			return 0, syscall.ENODATA
		}

		// For trusted.* and security.* attributes
		return 0, syscall.ENODATA
	}

	// Handle special cases for file capabilities
	if attr == "security.capability" {
		return 0, syscall.ENODATA
	}

	// Handle special case for "com.apple.ResourceFork" on macOS
	if attr == "com.apple.ResourceFork" {
		return 0, syscall.ENODATA
	}

	// For list size query (dest == nil), return minimal size
	if dest == nil {
		return 0, 0
	}

	// Log the attribute request for debugging
	log.Printf("Getxattr requested attr=%s for path=%s", attr, path)

	// Default case - attribute not supported
	return 0, syscall.ENODATA
}

// var _ = (NodeSetxattrer)((*LoopbackNode)(nil))

// func (n *LoopbackNode) Setxattr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
// 	err := unix.Lsetxattr(n.path(), attr, data, int(flags))
// 	return ToErrno(err)
// }

// var _ = (NodeRemovexattrer)((*LoopbackNode)(nil))

// func (n *LoopbackNode) Removexattr(ctx context.Context, attr string) syscall.Errno {
// 	err := unix.Lremovexattr(n.path(), attr)
// 	return ToErrno(err)
// }

// var _ = (NodeCopyFileRanger)((*LoopbackNode)(nil))

// func (n *LoopbackNode) CopyFileRange(ctx context.Context, fhIn FileHandle,
// 	offIn uint64, out *Inode, fhOut FileHandle, offOut uint64,
// 	len uint64, flags uint64) (uint32, syscall.Errno) {
// 	lfIn, ok := fhIn.(*loopbackFile)
// 	if !ok {
// 		return 0, unix.ENOTSUP
// 	}
// 	lfOut, ok := fhOut.(*loopbackFile)
// 	if !ok {
// 		return 0, unix.ENOTSUP
// 	}
// 	signedOffIn := int64(offIn)
// 	signedOffOut := int64(offOut)
// 	doCopyFileRange(lfIn.fd, signedOffIn, lfOut.fd, signedOffOut, int(len), int(flags))
// 	return 0, syscall.ENOSYS
// }

// NewLoopbackRoot returns a root node for a loopback file system whose
// root is at the given root. This node implements all NodeXxxxer
// operations available.
func NewLoopbackRoot(wt billy.Filesystem, config MountConfig) (fs.InodeEmbedder, error) {
	var st syscall.Stat_t
	err := syscall.Stat(wt.Root(), &st)
	if err != nil {
		return nil, err
	}

	root := &LoopbackRoot{
		mountConfig: config,
		wt:          wt,
		rootPath:    "",
		Dev:         uint64(st.Dev),
	}

	rootNode := root.newNode(nil, "", &st)
	root.RootNode = rootNode
	return rootNode, nil
}
