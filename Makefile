
GIT_REPO=git@github.com:tryy3/test-project2.git
MOUNT_POINT=/home/tryy3/Codes/Go/gittyfs/test-mnt

build:
	go build -o gittyfs cmd/main.go

unmount:
	fusermount -uz $(MOUNT_POINT)

run: build unmount
	./gittyfs -git $(GIT_REPO) -auth ~/.ssh/id_ed25519 -uid 65534 -gid 65534 $(MOUNT_POINT)
