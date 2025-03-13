package main

import (
	"crypto"
	"crypto/sha1"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/hash"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/tryy3/gittyfs/gittyfuse"
	"github.com/tryy3/gittyfs/manager"
)

func createRepository(url string, authFile string) (*git.Repository, error) {
	hash.RegisterHash(crypto.SHA1, sha1.New)
	// trace.SetTarget(trace.Packet)
	ep, err := transport.NewEndpoint(url)
	if err != nil {
		log.Fatalf("new endpoint %s: %s", url, err)
	}

	var authMethod transport.AuthMethod
	if authFile != "" {
		authMethod, err = ssh.NewPublicKeysFromFile(ep.User, authFile, "")
		if err != nil {
			log.Fatalf("new public keys from file %s: %s", authFile, err)
		}
	} else {
		authMethod, err = ssh.NewSSHAgentAuth(ep.User)
		if err != nil {
			log.Fatalf("new ssh agent auth %s: %s", ep.User, err)
		}
	}

	wt := memfs.New()
	storer := memory.NewStorage()
	r, err := git.Clone(storer, wt, &git.CloneOptions{
		Auth:         authMethod,
		URL:          url,
		Tags:         git.NoTags,
		Depth:        1,
		SingleBranch: true,
		// Progress:     os.Stdout,
	})

	if err != nil {
		log.Fatalf("git clone %s: %s", url, err)
	}

	return r, nil
}

// Mount path
// UID
// GID
// GIT

func main() {
	var gitURL string
	var UID string
	var GID string
	var authFile string
	var profile string
	var memProfile string
	var err error

	flag.StringVar(&gitURL, "git", "", "git url")
	flag.StringVar(&UID, "uid", "", "uid")
	flag.StringVar(&GID, "gid", "", "gid")
	flag.StringVar(&authFile, "auth", "", "auth file")
	flag.StringVar(&profile, "profile", "profile.pprof", "record cpu profile.")
	flag.StringVar(&memProfile, "mem-profile", "mem-profile.pprof", "record memory profile.")
	flag.Parse()

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <mount_path>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	// Parse flags
	flag.Parse()

	// Get positional arguments (after the flags)
	args := flag.Args()
	if len(args) < 1 {
		flag.Usage()
		log.Fatal("Error: Mount path is required")
	}

	if gitURL == "" {
		flag.Usage()
		log.Fatal("Error: Git URL is required")
	}

	mountPath := args[0]

	// Clone the given repository to the given directory
	log.Printf("git clone %s", gitURL)

	repo, err := createRepository(gitURL, authFile)
	if err != nil {
		log.Fatalf("git clone %s: %s", gitURL, err)
	}

	manager := manager.NewManager(repo, authFile)
	go manager.Run()

	wt, err := repo.Worktree()
	if err != nil {
		log.Fatalf("git worktree %s: %s", gitURL, err)
	}

	var uid uint32
	var gid uint32
	if UID != "" {
		uidInt, err := strconv.ParseUint(UID, 10, 32)
		if err != nil {
			log.Fatalf("invalid uid: %s", UID)
		}
		uid = uint32(uidInt)
	}
	if GID != "" {
		gidInt, err := strconv.ParseUint(GID, 10, 32)
		if err != nil {
			log.Fatalf("invalid gid: %s", GID)
		}
		gid = uint32(gidInt)
	}

	// var profFile, memProfFile *os.File
	// if profile != "" {
	// 	profFile, err = os.Create(profile)
	// 	if err != nil {
	// 		log.Fatalf("os.Create: %v", err)
	// 	}
	// }
	// if memProfile != "" {
	// 	memProfFile, err = os.Create(memProfile)
	// 	if err != nil {
	// 		log.Fatalf("os.Create: %v", err)
	// 	}
	// }
	// runtime.GC()
	// if profFile != nil {
	// 	log.Printf("start cpu profile")
	// 	pprof.StartCPUProfile(profFile)
	// 	defer pprof.StopCPUProfile()
	// }
	// Start profiling with 5-second flush interval
	profCtx, err := StartProfiling(
		"cpu_profile.prof",
		"mem_profile.prof",
		5*time.Second,
	)
	if err != nil {
		log.Printf("Failed to start profiling: %v", err)
		// Continue with mount even if profiling fails
	}
	// Set up signal handler to ensure profiling data is saved on clean shutdown
	setupSignalHandler(profCtx)

	go gittyfuse.MountLoopback(wt.Filesystem, mountPath, gittyfuse.MountConfig{
		UID:            uid,
		GID:            gid,
		FilePermission: 0644,
		DirPermission:  0755,
	})
	defer gittyfuse.UnmountLoopback()
	// fs := gittyfuse.NewFilesystem(wt.Filesystem, manager, UID, GID)
	// go fs.Mount(mountPath)
	// defer fs.Unmount()
	// go func() {
	// 	ticker := time.NewTicker(time.Millisecond * 10)
	// 	for {
	// 		select {
	// 		case <-ticker.C:
	// 			if profFile != nil {
	// 				profFile.Sync()
	// 			}
	// 			if memProfFile != nil {
	// 				pprof.WriteHeapProfile(memProfFile)
	// 			}
	// 		}
	// 	}
	// }()

	// defer func() {
	// 	if memProfFile != nil {
	// 		pprof.WriteHeapProfile(memProfFile)
	// 	}
	// }()

	select {}
}

// setupSignalHandler ensures profiling data is saved on clean shutdown
func setupSignalHandler(profCtx *ProfilingContext) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-c
		log.Printf("Received signal %v, stopping profiling before exit", sig)
		if profCtx != nil {
			profCtx.StopProfiling()
		}
		os.Exit(1)
	}()
}
