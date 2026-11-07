//go:build darwin || linux

package agentartifact

import (
	"io"
	"os"
	"strings"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"golang.org/x/sys/unix"
)

// ReadBound opens relative to the service root, refusing symlinks in every
// component and dangerous hard links. Verification/copy use the same open FD.
func ReadBound(root, relative string, maxBytes int64) ([]byte, error) {
	deny := func() ([]byte, error) { return nil, api.Failure(503, "artifact_access_blocked") }
	if relative == "" || strings.HasPrefix(relative, "/") || strings.ContainsAny(relative, "\\:\x00") || len(relative) > 4096 {
		return deny()
	}
	parts := strings.Split(relative, "/")
	if len(parts) > 32 {
		return deny()
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return deny()
		}
	}
	fd, e := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return deny()
	}
	for _, part := range parts[:len(parts)-1] {
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if err != nil {
			return deny()
		}
		fd = next
	}
	final, e := unix.Openat(fd, parts[len(parts)-1], unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	unix.Close(fd)
	if e != nil {
		return deny()
	}
	file := os.NewFile(uintptr(final), "bound-artifact")
	defer file.Close()
	var before, after unix.Stat_t
	if unix.Fstat(final, &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Size < 1 || before.Size > maxBytes {
		return deny()
	}
	data, e := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if e != nil || int64(len(data)) != before.Size || unix.Fstat(final, &after) != nil {
		return deny()
	}
	if before.Dev != after.Dev || before.Ino != after.Ino || before.Mode != after.Mode || before.Uid != after.Uid || before.Gid != after.Gid || before.Nlink != after.Nlink || before.Size != after.Size || before.Mtim != after.Mtim || before.Ctim != after.Ctim {
		return deny()
	}
	return data, nil
}
