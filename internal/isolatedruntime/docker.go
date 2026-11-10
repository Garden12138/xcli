package isolatedruntime

// This is an owner-configured, offline spike transport, not an authenticated
// Codex runtime. No Docker options, endpoint, image or paths come from HTTP.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/agentartifact"
)

var imageID = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var taskID = regexp.MustCompile(`^tsk_[a-f0-9]{32}$`)
var executableName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type Docker struct {
	binary, endpoint, config, workspace, image, executable, binaryHash, nonce string
	identity                                                                  os.FileInfo
	binaryTarget                                                              string
	binaryIdentity                                                            os.FileInfo
	mu                                                                        sync.Mutex
}
type DockerOptions struct{ Binary, Endpoint, ConfigDir, Workspace, Image, Executable, Nonce string }

func NewDocker(o DockerOptions) (*Docker, error) {
	if !imageID.MatchString(o.Image) || !executableName.MatchString(o.Executable) || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(o.Nonce) {
		return nil, errors.New("isolated_runtime_registration_invalid")
	}
	if !strings.HasPrefix(o.Endpoint, "unix:///") || strings.ContainsAny(o.Endpoint, "\x00\r\n") {
		return nil, errors.New("isolated_runtime_local_socket_required")
	}
	// OrbStack's registered docker entry is a multicall symlink: resolving it
	// as the invocation path changes argv[0] and no longer invokes Docker.
	if !filepath.IsAbs(o.Binary) || filepath.Clean(o.Binary) != o.Binary || strings.ContainsAny(o.Binary, "\x00\r\n,") {
		return nil, errors.New("isolated_runtime_binary_invalid")
	}
	binaryTarget, e := filepath.EvalSymlinks(o.Binary)
	if e != nil {
		return nil, e
	}
	binaryIdentity, e := os.Stat(binaryTarget)
	if e != nil || !binaryIdentity.Mode().IsRegular() || binaryIdentity.Mode().Perm()&0111 == 0 {
		return nil, errors.New("isolated_runtime_binary_invalid")
	}
	for _, path := range []string{o.ConfigDir, o.Workspace} {
		c, e := filepath.EvalSymlinks(path)
		if e != nil || !filepath.IsAbs(path) || c != filepath.Clean(path) || strings.ContainsAny(path, "\x00\r\n,") {
			return nil, errors.New("isolated_runtime_canonical_path_required")
		}
	}
	info, e := os.Stat(o.ConfigDir)
	if e != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("isolated_runtime_private_config_required")
	}
	entries, e := os.ReadDir(o.ConfigDir)
	if e != nil || len(entries) != 0 {
		return nil, errors.New("isolated_runtime_empty_docker_config_required")
	}
	ws, e := os.Lstat(o.Workspace)
	if e != nil || !ws.IsDir() || o.Workspace == "/" || o.Workspace == o.ConfigDir || strings.HasPrefix(o.ConfigDir, o.Workspace+string(filepath.Separator)) {
		return nil, errors.New("isolated_runtime_workspace_invalid")
	}
	// A public synthetic child is safe only under its private owner directory.
	parent, e := os.Stat(filepath.Dir(o.Workspace))
	if e != nil || parent.Mode().Perm()&0077 != 0 {
		return nil, errors.New("isolated_runtime_private_workspace_parent_required")
	}
	b, e := agentartifact.ReadBound(o.Workspace, o.Executable, 512<<20)
	if e != nil {
		return nil, e
	}
	sum := sha256.Sum256(b)
	return &Docker{binary: o.Binary, binaryTarget: binaryTarget, binaryIdentity: binaryIdentity, endpoint: o.Endpoint, config: o.ConfigDir, workspace: o.Workspace, image: o.Image, executable: o.Executable, binaryHash: hex.EncodeToString(sum[:]), nonce: o.Nonce, identity: ws}, nil
}
func (d *Docker) binaryUnchanged() bool {
	target, e := filepath.EvalSymlinks(d.binary)
	if e != nil || target != d.binaryTarget {
		return false
	}
	info, e := os.Stat(target)
	return e == nil && os.SameFile(info, d.binaryIdentity) && info.Size() == d.binaryIdentity.Size() && info.ModTime() == d.binaryIdentity.ModTime() && info.Mode() == d.binaryIdentity.Mode()
}
func (d *Docker) Acquire() bool { return d.mu.TryLock() }
func (d *Docker) Release()      { d.mu.Unlock() }
func (d *Docker) Name(task string) (string, error) {
	if !taskID.MatchString(task) {
		return "", errors.New("isolated_runtime_task_invalid")
	}
	return "xcli-g7-" + d.nonce + "-" + task[4:], nil
}
func (d *Docker) Environment() []string {
	return []string{"HOME=" + d.config, "DOCKER_CONFIG=" + d.config, "PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin", "LANG=C.UTF-8"}
}
func (d *Docker) baseArgs() []string { return []string{"--host", d.endpoint, "--config", d.config} }
func (d *Docker) Spec(task string, args []string) (agent.CommandSpec, string, []string, error) {
	if !d.binaryUnchanged() {
		return agent.CommandSpec{}, "", nil, errors.New("isolated_runtime_binary_changed")
	}
	name, e := d.Name(task)
	if e != nil {
		return agent.CommandSpec{}, "", nil, e
	}
	current, e := os.Lstat(d.workspace)
	if e != nil || !os.SameFile(current, d.identity) || current.Mode()&os.ModeSymlink != 0 {
		return agent.CommandSpec{}, "", nil, errors.New("isolated_runtime_workspace_changed")
	}
	b, e := agentartifact.ReadBound(d.workspace, d.executable, 512<<20)
	if e != nil {
		return agent.CommandSpec{}, "", nil, e
	}
	hash := sha256.Sum256(b)
	if hex.EncodeToString(hash[:]) != d.binaryHash {
		return agent.CommandSpec{}, "", nil, errors.New("isolated_runtime_executable_changed")
	}
	command := append(d.baseArgs(), "run", "--rm", "--pull=never", "--platform=linux/arm64", "--name", name, "--label", "xcli.g7.owner="+d.nonce, "--network=none", "--read-only", "--user=65532:65532", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=512m", "--cpus=1", "--ipc=none", "--interactive", "--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=16m,mode=1777", "--tmpfs", "/home/xcli:rw,noexec,nosuid,nodev,size=32m,mode=0700,uid=65532,gid=65532", "--mount", "type=bind,src="+d.workspace+",dst=/workspace,readonly", "--workdir=/workspace", "--env", "HOME=/home/xcli", "--env", "CODEX_HOME=/home/xcli", "--env", "PATH=/usr/bin:/bin", "--env", "LANG=C.UTF-8", "--entrypoint", "/workspace/"+d.executable, d.image)
	command = append(command, args...)
	return agent.CommandSpec{Command: d.binary, Args: command}, d.config, d.Environment(), nil
}

// A stopped Docker client is not proof that the daemon-side workload stopped.
// Query only our exact name AND registration label; never remove other jobs.
func (d *Docker) StopConfirmed(task string) bool {
	name, e := d.Name(task)
	if e != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	list := func() (string, bool) {
		args := append(d.baseArgs(), "container", "ls", "--all", "--filter", "name=^/"+name+"$", "--filter", "label=xcli.g7.owner="+d.nonce, "--format", "{{.ID}}")
		out, ok := d.control(ctx, args)
		return strings.TrimSpace(out), ok
	}
	id, ok := list()
	if !ok {
		return false
	}
	if id == "" {
		return true
	}
	if !regexp.MustCompile(`^[a-f0-9]{12,64}$`).MatchString(id) {
		return false
	}
	if _, ok = d.control(ctx, append(d.baseArgs(), "container", "rm", "--force", id)); !ok {
		return false
	}
	id, ok = list()
	return ok && id == ""
}
func (d *Docker) control(ctx context.Context, args []string) (string, bool) {
	if !d.binaryUnchanged() {
		return "", false
	}
	cmd := exec.CommandContext(ctx, d.binary, args...)
	cmd.Dir = d.config
	cmd.Env = d.Environment()
	cmd.WaitDelay = 100 * time.Millisecond
	var out controlBuffer
	cmd.Stdout = &out
	e := cmd.Run()
	return out.String(), e == nil && !out.truncated
}

type controlBuffer struct {
	bytes.Buffer
	truncated bool
}

func (b *controlBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 4096 - b.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	_, e := b.Buffer.Write(p)
	return n, e
}
