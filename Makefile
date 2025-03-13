
GIT_REPO=git@github.com:tryy3/test-project2.git
MOUNT_POINT=/home/tryy3/Codes/Go/gittyfs/test-mnt

build:
	go build -o gittyfs cmd/*.go

unmount:
	fusermount -uz $(MOUNT_POINT)

run: build unmount
	./gittyfs -git $(GIT_REPO) -auth ~/.ssh/id_ed25519 -uid 1000 -gid 1000 $(MOUNT_POINT)

test1:
	mv test-mnt/foo2/yello/yesbox.txt test-mnt/foo/derp/foobar.txt
	ls -la test-mnt/foo/derp
	ls -la test-mnt/foo2/yello

test2:
	mv test-mnt/foo/derp/foobar.txt test-mnt/foo2/yello/yesbox.txt
	ls -la test-mnt/foo/derp
	ls -la test-mnt/foo2/yello

test3:
	mv test-mnt/yes.txt test-mnt/no.txt
	ls -la test-mnt

test4:
	mv test-mnt/no.txt test-mnt/yes.txt
	ls -la test-mnt

test5:
	mv test-mnt/foo/hello.txt test-mnt/foo/hello2.txt
	ls -la test-mnt/foo

test6:
	mv test-mnt/foo/hello2.txt test-mnt/foo/hello.txt
	ls -la test-mnt/foo
