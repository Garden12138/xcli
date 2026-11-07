// Package agentartifact owns bounded immutable byte storage and safe import.
package agentartifact

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/media"
	"github.com/Garden12138/xcli/internal/modelstore"
)

const StorageLimit = 256 << 20

var id = regexp.MustCompile(`^(inp|art)_[a-f0-9]{32}$`)

func blobName(kind, object string) (string, error) {
	if !id.MatchString(object) || (kind != "inputs" && kind != "artifacts") {
		return "", api.Failure(404, "not_found")
	}
	return filepath.Join("blobs", kind, object+".blob"), nil
}
func Put(root, kind, object string, data []byte) error {
	name, e := blobName(kind, object)
	if e != nil {
		return e
	}
	base := root
	for _, component := range []string{"blobs", kind} {
		base = filepath.Join(base, component)
		if e = modelstore.PrivateDirectory(base); e != nil {
			return e
		}
	}
	total := int64(0)
	e = filepath.WalkDir(filepath.Join(root, "blobs"), func(path string, entry fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return api.Failure(503, "store_unavailable")
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if e != nil {
		return e
	}
	if total+int64(len(data)) > StorageLimit {
		return api.Failure(503, "store_unavailable")
	}
	file, e := os.CreateTemp(base, ".import-")
	if e != nil {
		return e
	}
	temp := file.Name()
	defer os.Remove(temp)
	if _, e = file.Write(data); e != nil {
		file.Close()
		return e
	}
	if e = file.Chmod(0o400); e != nil {
		file.Close()
		return e
	}
	if e = file.Sync(); e != nil {
		file.Close()
		return e
	}
	if e = file.Close(); e != nil {
		return e
	}
	dest := filepath.Join(root, name)
	if _, e = os.Lstat(dest); e == nil || !os.IsNotExist(e) {
		return api.Failure(503, "store_unavailable")
	}
	if e = os.Rename(temp, dest); e != nil {
		return e
	}
	dir, e := os.Open(base)
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
func Read(root, kind, object string, max int64) ([]byte, error) {
	name, e := blobName(kind, object)
	if e != nil {
		return nil, e
	}
	return ReadBound(root, filepath.ToSlash(name), max)
}
func Remove(root, kind, object string) error {
	name, e := blobName(kind, object)
	if e != nil {
		return e
	}
	e = os.Remove(filepath.Join(root, name))
	if os.IsNotExist(e) {
		return nil
	}
	return e
}
func Validate(data []byte, claimed string) (media.ImageInfo, error) {
	info, e := media.ValidateImage(data, claimed)
	if e == nil {
		return info, nil
	}
	var bad *media.Error
	if !errors.As(e, &bad) {
		return info, api.Failure(422, "invalid_image")
	}
	code := bad.Code
	if code == "image_too_large" {
		code = "input_too_large"
	}
	return info, api.Failure(bad.Status, code)
}
