package tarski

import (
	"io"
	"os"
	"archive/tar"
	"testing"
)

const archive string = "test.tar"

func TestCreate(t *testing.T) {
	var err error
	expect := []string{
		"Dir/",
		"Dir/somefile.txt",
		"xattrs.txt",
		"xattrs_symlink",
	}

	if err = Create(archive, "testdata", "testdata"); err != nil {
		os.Remove("test.tar")
		t.Fatal(err)
	}

	if _, err = os.Stat(archive); os.IsNotExist(err) {
		os.Remove("test.tar")
		t.Fatal(err)
	}

	f, err := os.Open(archive)
	if err != nil {
		os.Remove("test.tar")
		t.Fatal(err)
	}

	r := tar.NewReader(f)

	var i int
	for h, err := r.Next(); err != io.EOF; h, err = r.Next() {
		if err != nil {
			f.Close()
			os.Remove("test.tar")
			t.Fatal(err)
		}
		if expect[i] != h.Name {
			f.Close()
			os.Remove("test.tar")
			t.Fatal(err)
		}
		i++
	}

	if err = f.Close(); err != nil {
		os.Remove("test.tar")
		t.Log(err)
	}

	if err = os.Remove(archive); err != nil {
		t.Log(err)
	}
}
