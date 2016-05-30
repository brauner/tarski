package tarski

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"golang.org/x/sys/unix"
	"io"
	"log"
	"os"
	"testing"
)

const archive string = "test.tar"
const extractArchive string = "testExtract"

var prefix = "testdata/"
var entries = []string{
	"Dir/",
	"Dir/somefile",
	"hard",
	"hard_link",
	"sym",
	"sym_link",
	"xattrs",
}
var testxattr = map[string]string{
	"user.checksum": "asdfsf13434qwf1324",
	"user.random":   "This is a test",
}

func setup() error {
	// "Dir/"
	err := os.MkdirAll(prefix+entries[0], 0755)
	if err != nil {
		return err
	}

	// "Dir/somefile"
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

	// "hard"
	f, err = os.OpenFile(prefix+entries[2], os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}

	_, err = f.WriteString("This is a regular file to serve as the target of a hard link.")
	if err != nil {
		f.Close()
		return err
	}
	f.Close()

	// "hard_link"
	if err = os.Link(prefix+entries[2], prefix+entries[3]); err != nil {
		return err
	}

	// "sym"
	f, err = os.OpenFile(prefix+entries[4], os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}

	_, err = f.WriteString("This is a regular file to serve as the target of a symbolic link.")
	if err != nil {
		f.Close()
		return err
	}
	f.Close()

	// "sym_link"
	if err = os.Symlink(prefix+entries[4], prefix+entries[5]); err != nil {
		return err
	}

	// "xattrs"
	f, err = os.OpenFile(prefix+entries[6], os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}

	_, err = f.WriteString("This file is used to test whether extended attributes are preserved when taring and untaring.")
	if err != nil {
		f.Close()
		return err
	}
	f.Close()

	for k, v := range testxattr {
		if err = unix.Setxattr(prefix+entries[0], k, []byte(v), 0); err != nil {
			f.Close()
			return err
		}
		if err = unix.Setxattr(prefix+entries[6], k, []byte(v), 0); err != nil {
			f.Close()
			return err
		}
	}

	return nil
}

func cleanup() {
	os.Remove(archive)
	os.RemoveAll(prefix)
	os.RemoveAll(extractArchive)
}

func TestMain(m *testing.M) {
	if err := setup(); err != nil {
		log.Println(err)
	}
	res := m.Run()
	cleanup()
	os.Exit(res)
}

func TestCreate(t *testing.T) {
	var err error

	if _, err = os.Stat(archive); err == nil {
		os.Remove(archive)
	}

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

	// Test retrieval of extended attributes for regular files.
	h, err := GetAllXattr(prefix + entries[6])
	if err != nil {
		t.Fatal(err)
	}

	if h == nil {
		t.Fatalf("Expected to find extended attributes but did not find any.")
	}

	for k, v := range h {
		found, ok := h[k]
		if !ok || found != testxattr[k] {
			t.Fatalf("Expected to find extended attribute %s with a value of %s on regular file but did not find it.", k, v)
		}
	}

	// Test retrieval of extended attributes for directories.
	h, err = GetAllXattr(prefix + entries[0])
	if err != nil {
		t.Fatal(err)
	}

	if h == nil {
		t.Fatalf("Expected to find extended attributes but did not find any.")
	}

	for k, v := range h {
		found, ok := h[k]
		if !ok || found != testxattr[k] {
			t.Fatalf("Expected to find extended attribute %s with a value of %s on directory but did not find it.", k, v)
		}
	}
}

func TestCreateSHA256(t *testing.T) {
	var err error
	if _, err = os.Stat(archive); err == nil {
		os.Remove(archive)
	}

	checksum, err := CreateSHA256(archive, prefix, prefix)
	if err != nil {
		t.Fatal(err)
	}
	calculatedChecksum := hex.EncodeToString(checksum)

	if _, err = os.Stat(archive); os.IsNotExist(err) {
		t.Fatal(err)
	}

	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}

	s := sha256.New()
	if _, err = io.Copy(s, f); err != nil {
		t.Fatal(err)
	}
	expectedChecksum := hex.EncodeToString(s.Sum(nil))

	var i int
	r := tar.NewReader(f)
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

	if calculatedChecksum != expectedChecksum {
		t.Fatalf("Expected checksum %s. Received %s instead.", expectedChecksum, hex.EncodeToString(checksum))
	}
}

func TestExtract(t *testing.T) {
	if _, err := os.Stat(archive); err == nil {
		os.Remove(archive)
	}

	err := Create(archive, prefix, prefix)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = os.Stat(archive); os.IsNotExist(err) {
		t.Fatal(err)
	}

	if _, err := os.Stat(extractArchive); err == nil {
		os.RemoveAll(extractArchive)
	}

	err = Extract(archive, extractArchive)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = os.Stat(extractArchive); os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestExtractSHA256(t *testing.T) {
	if _, err := os.Stat(archive); err == nil {
		os.Remove(archive)
	}

	checksum, err := CreateSHA256(archive, prefix, prefix)
	if err != nil {
		t.Fatal(err)
	}
	expectedChecksum := hex.EncodeToString(checksum)

	if _, err = os.Stat(archive); os.IsNotExist(err) {
		t.Fatal(err)
	}

	if _, err := os.Stat(extractArchive); err == nil {
		os.RemoveAll(extractArchive)
	}

	checksum, err = ExtractSHA256(archive, extractArchive)
	if err != nil {
		t.Fatal(err)
	}
	calculatedChecksum := hex.EncodeToString(checksum)

	if expectedChecksum != calculatedChecksum {
		t.Fatal(err)
	}
}
