package modelstore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func checkPrivate(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) {
		return fmt.Errorf("private state must be a regular file or directory, without symlinks")
	}
	return checkPermissions(path, info)
}

func PrivateDirectory(path string) error {
	_, err := os.Lstat(path)
	fresh := os.IsNotExist(err)
	if err != nil && !fresh {
		return err
	}
	if err = os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if fresh {
		if err = secureNewDirectory(path); err != nil {
			return err
		}
	}
	return checkPrivate(path, true)
}

func ReadPrivate(path string, max int64) ([]byte, error) {
	if err := checkPrivate(path, false); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errors.New("private file exceeds limit")
	}
	return b, nil
}

// WritePrivate atomically replaces an existing private file, never follows its symlink.
func WritePrivate(path string, data []byte) error {
	if err := checkPrivate(filepath.Dir(path), true); err != nil {
		return err
	}
	if err := checkPrivate(path, false); err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".xcli-state-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

// ResolveSecret never consults native CLI login files or the legacy Agent config.
func ResolveSecret(ref, base string) (string, error) {
	if ref == "" {
		return "", nil
	}
	var value string
	switch {
	case strings.HasPrefix(ref, "env:"):
		value = os.Getenv(strings.TrimPrefix(ref, "env:"))
	case strings.HasPrefix(ref, "file:"):
		path := strings.TrimPrefix(ref, "file:")
		if !filepath.IsAbs(path) {
			path = filepath.Join(base, path)
		}
		b, err := ReadPrivate(path, 16<<10)
		if err != nil {
			return "", errors.New("cannot read private upstream credential file")
		}
		value = strings.TrimSpace(string(b))
	default:
		return "", errors.New("credential reference must use env: or file:")
	}
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return "", errors.New("upstream credential reference is empty or invalid")
	}
	return value, nil
}
