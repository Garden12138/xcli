package modelstore

import (
	"errors"
	"github.com/Garden12138/xcli/internal/localstate"
	"os"
	"path/filepath"
	"strings"
)

func checkPrivate(p string, d bool) error           { return localstate.CheckPrivate(p, d) }
func PrivateDirectory(p string) error               { return localstate.PrivateDirectory(p) }
func ReadPrivate(p string, n int64) ([]byte, error) { return localstate.ReadPrivate(p, n) }
func WritePrivate(p string, b []byte) error         { return localstate.WritePrivate(p, b) }

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
