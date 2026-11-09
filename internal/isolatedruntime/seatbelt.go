// Package isolatedruntime provides a credential-free, offline whole-process
// boundary. It is not an authenticated model runtime or a user namespace.
package isolatedruntime

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Garden12138/xcli/internal/agent"
)

func Seatbelt(binary, workspace, home string, args []string) (agent.CommandSpec, error) {
	if runtime.GOOS != "darwin" {
		return agent.CommandSpec{}, errors.New("outer_isolation_platform_unverified")
	}
	for _, p := range []string{binary, workspace, home} {
		canonical, e := filepath.EvalSymlinks(p)
		if e != nil || !filepath.IsAbs(p) || canonical != filepath.Clean(p) || strings.ContainsAny(p, "\x00\r\n") {
			return agent.CommandSpec{}, errors.New("outer_isolation_path_invalid")
		}
	}
	// This prototype only accepts sibling private disposable directories; it
	// cannot accidentally grant the filesystem root or a personal HOME tree.
	if home == "/" || workspace == "/" || filepath.Dir(home) != filepath.Dir(workspace) {
		return agent.CommandSpec{}, errors.New("outer_isolation_private_siblings_required")
	}
	for _, p := range []string{home, workspace, filepath.Dir(home)} {
		info, e := os.Stat(p)
		if e != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
			return agent.CommandSpec{}, errors.New("outer_isolation_private_directories_required")
		}
	}
	b, e := os.Stat(binary)
	if e != nil || !b.Mode().IsRegular() {
		return agent.CommandSpec{}, errors.New("outer_isolation_binary_invalid")
	}
	// Do not allow home to contain the workspace, executable or any caller's
	// personal directory. The caller creates this empty disposable home itself.
	if home == workspace || strings.HasPrefix(workspace, home+string(os.PathSeparator)) || strings.HasPrefix(binary, home+string(os.PathSeparator)) {
		return agent.CommandSpec{}, errors.New("outer_isolation_roots_overlap")
	}
	quote := strconv.Quote
	policy := `(version 1)
(deny default)
(allow process-fork)
(allow process-exec (literal ` + quote(binary) + `) (subpath "/bin") (subpath "/usr/bin") (subpath "/usr/libexec"))
(allow signal (target self))
(allow sysctl-read)
(allow file-read-metadata)
(allow file-read* (literal ` + quote(binary) + `) (subpath "/usr/lib") (subpath "/System/Library") (subpath "/Library/Apple/System/Library") (subpath ` + quote(workspace) + `) (subpath ` + quote(home) + `) (literal "/dev/null") (literal "/dev/random") (literal "/dev/urandom") (literal "/private/etc/localtime"))
(allow file-write* (subpath ` + quote(home) + `) (literal "/dev/null"))
`
	// No network or Mach/Keychain service grants. All descendants inherit this
	// kernel boundary; model and authentication connectivity are intentionally off.
	return agent.CommandSpec{Command: "/usr/bin/sandbox-exec", Args: append([]string{"-p", policy, binary}, args...)}, nil
}
