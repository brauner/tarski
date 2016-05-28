package tarski

import (
	"archive/tar"
	"golang.org/x/sys/unix"
	"io"
	"log"
	"os"
	"testing"
)

const archive string = "test.tar"

var prefix string = "testdata/"
var entries = []string{
	"Dir/",
	"Dir/somefile.txt",
	"xattrs.txt",
	"xattrs_symlink",
}
var testxattr = map[string]string{
	"user.checksum": "asdfsf13434qwf1324",
	"user.random":   "This is a test",
}

func setup() error {
	err := os.MkdirAll(prefix+entries[0], 0755)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(prefix+entries[1], os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}

	_, err = f.WriteString("This is a regular file.")
	if err != nil {
		f.Close()
		return err
	}
	f.Close()

	f, err = os.OpenFile(prefix+entries[2], os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}

	_, err = f.WriteString("This file is used to test whether extended attributes are preserved when taring and untaring.")
	if err != nil {
		f.Close()
		return err
	}

	for k, v := range testxattr {
		if err = unix.Setxattr(prefix+entries[2], k, []byte(v), 0); err != nil {
			f.Close()
			return err
		}
	}
	f.Close()

	if err = os.Symlink(prefix+entries[2], prefix+entries[3]); err != nil {
		return err
	}

	return nil
}

func cleanup() {
	os.Remove("test.tar")
	os.RemoveAll(prefix)
}

func TestMain(m *testing.M) {
	if err := setup(); err != nil {
		log.Println("Failed to setup test environment.")
	}
	res := m.Run()
	cleanup()
	os.Exit(res)
}

func TestCreate(t *testing.T) {
	var err error

	if err = Create(archive, prefix, prefix); err != nil {
		t.Fatal(err)
	}

	if _, err = os.Stat(archive); os.IsNotExist(err) {
		t.Fatal(err)
	}

	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}

	r := tar.NewReader(f)

	var i int
	for h, err := r.Next(); err != io.EOF; h, err = r.Next() {
		if err != nil {
			f.Close()
			t.Fatal(err)
		}
		if entries[i] != h.Name {
			f.Close()
			t.Fatal(err)
		}
		i++
	}

	if err = f.Close(); err != nil {
		t.Log(err)
	}
}

func TestGetAllXattr(t *testing.T) {
	var err error

	h, err := GetAllXattr(prefix + entries[2])
	if err != nil {
		t.Fatal(err)
	}

	if h == nil {
		t.Fatalf("Expected to find extended attributes but did not find any.")
	}

	for k, v := range h {
		found, ok := h[k]
		if !ok || found != testxattr[k] {
			t.Fatalf("Expected to find extended attribute %s with a value of %s but did not find it.", k, v)
		}
	}
}
