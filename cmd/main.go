package main

import (
	"crypto"
	"crypto/sha1"
	"flag"
	"fmt"
	"log"
	"os"

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

	flag.StringVar(&gitURL, "git", "", "git url")
	flag.StringVar(&UID, "uid", "", "uid")
	flag.StringVar(&GID, "gid", "", "gid")
	flag.StringVar(&authFile, "auth", "", "auth file")
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

	fs := gittyfuse.NewFilesystem(wt.Filesystem, manager, UID, GID)
	go fs.Mount(mountPath)
	defer fs.Unmount()

	select {}
}
