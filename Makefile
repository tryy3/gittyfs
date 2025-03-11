
GIT_REPO=git@github.com:tryy3/test-project2.git
MOUNT_POINT=/home/tryy3/Codes/Go/gittyfs/test-mnt

build:
	go build -o gittyfs cmd/main.go

run: build 
	./gittyfs -git $(GIT_REPO) -auth ~/.ssh/id_ed25519 $(MOUNT_POINT)
	fusermount -u $(MOUNT_POINT)
