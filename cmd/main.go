package main

import (
	"crypto"
	"crypto/sha1"
	"log"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/hash"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/tryy3/gittyfs/gittyfuse"
	"github.com/tryy3/gittyfs/manager"
)

func createRepository(url string) (*git.Repository, error) {
	hash.RegisterHash(crypto.SHA1, sha1.New)
	// trace.SetTarget(trace.Packet)

	authMethod, err := ssh.NewSSHAgentAuth("git")
	if err != nil {
		log.Fatalf("ssh agent auth %s: %s", url, err)
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

func main() {
	url := "git@github.com:tryy3/test-project.git"

	// Clone the given repository to the given directory
	log.Printf("git clone %s", url)

	repo, err := createRepository(url)
	if err != nil {
		log.Fatalf("git clone %s: %s", url, err)
	}

	manager := manager.NewManager(repo)
	go manager.Run()

	wt, err := repo.Worktree()
	if err != nil {
		log.Fatalf("git worktree %s: %s", url, err)
	}

	fs := gittyfuse.NewFilesystem(wt.Filesystem, manager)
	go fs.Mount("/home/tryy3/Codes/Go/gittyfs/test-mnt")
	defer fs.Unmount()

	select {}
}
