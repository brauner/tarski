package tarski

import (
	"archive/tar"
	"crypto/sha256"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

func IsEmpty(archive string) (bool, error) {
	f, err := os.Open(archive)
	if err != nil {
		return false, err
	}
	defer f.Close()

	t := tar.NewReader(f)
	_, err = t.Next()
	if err == io.EOF {
		return true, nil
	}

	return false, err
}

func CreateSHA256(archive string, path string, prefix string) (checksum []byte, err error) {
	a, err := os.Create(archive)
	if err != nil {
		return
	}
	defer a.Close()

	b := sha256.New()
	c := io.MultiWriter(a, b)
	d := tar.NewWriter(c)

	err = Readdir(d, path, prefix)
	if err != nil {
		return
	}

	if err = d.Close(); err != nil {
		return
	}

	checksum = b.Sum(nil)
	return
}

func Create(archive string, path string, prefix string) (err error) {
	f, err := os.Create(archive)
	if err != nil {
		return
	}
	defer f.Close()

	w := tar.NewWriter(f)

	err = Readdir(w, path, prefix)
	if err = w.Close(); err != nil {
		return
	}

	return
}

func WriteHeader(w *tar.Writer, path string, entry string, f os.FileInfo) (err error) {
	var link string

	if f.Mode()&os.ModeSymlink == os.ModeSymlink {
		link, err = os.Readlink(path)
		if err != nil {
			return
		}
	}

	h, err := tar.FileInfoHeader(f, link)
	if err != nil {
		return
	}

	h.Name = entry
	h.Xattrs, err = GetAllXattr(path)
	if err != nil {
		return
	}

	return w.WriteHeader(h)
}

func CopyContent(w *tar.Writer, path string) (err error) {
	f, err := os.Open(path)
	if err != nil {
		return
	}

	if _, err = io.Copy(w, f); err != nil {
		return
	}

	if err = f.Close(); err != nil {
		return
	}

	return
}

func cleanEntry(f os.FileInfo, path string, prefix string) (entry string) {
	entry = strings.TrimPrefix(path, prefix)
	if entry == "" || entry == "/" {
		return
	}

	if entry[0:1] == "/" {
		entry = entry[1:]
	}

	if f.IsDir() && (entry[len(entry)-1:len(entry)] != "/") {
		entry = entry + "/"
	}

	return
}

func Readdir(w *tar.Writer, path string, prefix string) error {
	return filepath.Walk(path, func(curpath string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		s := cleanEntry(f, curpath, prefix)
		if s == "" {
			return nil
		}

		mode := f.Mode()
		if err := WriteHeader(w, curpath, s, f); err != nil {
			return err
		}

		if (mode&os.ModeSymlink == os.ModeSymlink) || (mode&os.ModeDevice == os.ModeDevice) || f.IsDir() {
			return nil
		}

		if err := CopyContent(w, curpath); err != nil {
			return err
		}

		return nil
	})
}

func Extract(archive string, path string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()

	r := tar.NewReader(f)
	if err != nil {
		return err
	}

	for h, err := r.Next(); err != io.EOF; h, err = r.Next() {
		if err != nil {
			break
		}

		if h.Typeflag == tar.TypeDir {
			if err := ExtractDir(path, h); err != nil {
				return err
			}
		} else if h.Typeflag == tar.TypeSymlink {
			if err := ExtractSymlink(path, h); err != nil {
				return err
			}
		} else if h.Typeflag == tar.TypeChar || h.Typeflag == tar.TypeBlock {
			if err := ExtractDev(path, h); err != nil {
				return err
			}
		} else {
			if err := ExtractReg(path, h, r); err != nil {
				return err
			}
		}
	}

	return err
}

func ExtractDir(path string, h *tar.Header) (err error) {
	entry := filepath.Join(path, h.Name)
	fi := h.FileInfo()

	err = os.MkdirAll(entry, fi.Mode())
	if err != nil {
		return
	}

	if err = os.Chown(entry, h.Uid, h.Gid); err != nil {
		return
	}

	for attr, data := range h.Xattrs {
		if err = unix.Setxattr(entry, attr, []byte(data), 0); err != nil {
			return
		}
	}

	if err = os.Chtimes(entry, time.Now(), fi.ModTime()); err != nil {
		return
	}

	return
}

func ExtractReg(path string, h *tar.Header, r *tar.Reader) (err error) {
	fi := h.FileInfo()
	entry := filepath.Join(path, h.Name)
	filedir := filepath.Join(path, filepath.Dir(h.Name))

	err = os.MkdirAll(filedir, fi.Mode())
	if err != nil {
		return
	}

	g, err := os.OpenFile(entry, os.O_EXCL|os.O_WRONLY|os.O_CREATE, fi.Mode())
	if err != nil {
		return
	}

	w, err := io.Copy(g, r)
	if err != nil {
		return
	}
	if w != fi.Size() {
		return fmt.Errorf("Expected to write %d bytes, only wrote %d\n", fi.Size(), w)
	}
	if w != h.Size {
		return
	}

	if err = g.Close(); err != nil {
		return
	}

	if err := os.Chown(entry, h.Uid, h.Gid); err != nil {
		return err
	}

	for attr, data := range h.Xattrs {
		if err = unix.Setxattr(entry, attr, []byte(data), 0); err != nil {
			return err
		}
	}

	if err = os.Chtimes(entry, fi.ModTime(), fi.ModTime()); err != nil {
		return err
	}

	return
}

func ExtractSymlink(path string, h *tar.Header) (err error) {
	fi := h.FileInfo()
	entry := filepath.Join(path, h.Name)
	filedir := filepath.Join(path, filepath.Dir(h.Name))

	err = os.MkdirAll(filedir, fi.Mode())
	if err != nil {
		return
	}

	if err = os.Symlink(h.Linkname, entry); err != nil {
		return
	}

	if err = os.Lchown(entry, h.Uid, h.Gid); err != nil {
		return
	}

	var times = make([]unix.Timespec, 2)
	times[0].Sec = time.Now().Unix()
	times[1].Sec = fi.ModTime().Unix()
	err = unix.UtimesNanoAt(unix.AT_FDCWD, entry, times, unix.AT_SYMLINK_NOFOLLOW)
	if err != nil {
		return err
	}

	return
}

func ExtractDev(path string, h *tar.Header) (err error) {
	fi := h.FileInfo()
	entry := filepath.Join(path, h.Name)
	filedir := filepath.Join(path, filepath.Dir(h.Name))

	err = os.MkdirAll(filedir, fi.Mode())
	if err != nil {
		return
	}

	g, err := os.OpenFile(entry, os.O_EXCL|os.O_WRONLY|os.O_CREATE, fi.Mode())
	if err != nil {
		return
	}

	if err = g.Close(); err != nil {
		return
	}

	if err = os.Chown(entry, h.Uid, h.Gid); err != nil {
		return
	}

	if err = os.Chtimes(entry, fi.ModTime(), fi.ModTime()); err != nil {
		return err
	}

	return
}

// This uses ssize_t llistxattr(const char *path, char *list, size_t size); to
// handle symbolic links (should it in the future be possible to set extended
// attributed on symlinks): If path is a symbolic the extended attributes
// associated with the link itself are retrieved.
func llistxattr(path string, list []byte) (sz int, err error) {
	var _p0 *byte
	_p0, err = unix.BytePtrFromString(path)
	if err != nil {
		return
	}
	var _p1 unsafe.Pointer
	if len(list) > 0 {
		_p1 = unsafe.Pointer(&list[0])
	} else {
		_p1 = unsafe.Pointer(nil)
	}
	r0, _, e1 := unix.Syscall(unix.SYS_LLISTXATTR, uintptr(unsafe.Pointer(_p0)), uintptr(_p1), uintptr(len(list)))
	sz = int(r0)
	if e1 != 0 {
		err = e1
	}
	return
}

func GetAllXattr(path string) (xattrs map[string]string, err error) {
	sz, err := llistxattr(path, nil)
	if err != nil {
		return nil, err
	}
	if sz < 0 {
		return nil, err
	}
	if sz == 0 {
		return nil, nil
	}

	dest := make([]byte, sz)
	sz, err = llistxattr(path, dest)
	if err != nil {
		return nil, err
	}
	split := strings.Split(string(dest), "\x00")
	if split == nil {
		return nil, err
	}
	// *listxattr functions return a list of  names  as  an unordered array
	// of null-terminated character strings (attribute names are separated
	// by null bytes ('\0')), like this: user.name1\0system.name1\0user.name2\0
	// Since we split at the '\0'-byte the last element of the slice will be
	// the empty string. We remove it:
	if split[len(split)-1] == "" {
		split = split[:len(split)-1]
	}

	xattrs = make(map[string]string, len(split))

	for _, x := range split {
		xattr := string(x)
		sz, err = unix.Getxattr(path, xattr, nil)
		if err != nil {
			return nil, err
		}
		if sz < 0 {
			return nil, err
		}
		if sz == 0 {
			return nil, nil
		}

		val := make([]byte, sz)
		_, err = unix.Getxattr(path, xattr, val)
		if err != nil {
			return nil, err
		}
		xattrs[xattr] = string(val)
	}

	return xattrs, nil
}
