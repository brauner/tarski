package tarski

import (
	"golang.org/x/sys/unix"
	"testing"
)

const file string = "testdata/xattrs.txt"

func TestGetAllXattr(t *testing.T) {
	var err error
	xattr := "user.checksum"
	val := "asdfsf13434qwf1324"

	if err = unix.Setxattr(file, xattr,
	[]byte(val), 0); err != nil {
		t.Fatal(err)
	}

	h, err := GetAllXattr(file)
	if err != nil {
		t.Fatal(err)
	}

	if h == nil {
		t.Fatalf("Expected to find extended attributes but did not find any.")
	}

	found, ok := h[xattr]
	if !ok || found != val {
		t.Fatalf("Expected to find extended attribute %s with a value of %s but did not find it.", xattr, val)
	}
}
