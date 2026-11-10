package isolatedruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dockerFixture(t *testing.T, body string) (DockerOptions, *Docker) {
	t.Helper()
	base, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	os.Chmod(base, 0700)
	config, ws := filepath.Join(base, "docker-config"), filepath.Join(base, "workspace")
	for _, dir := range []string{config, ws} {
		if e = os.Mkdir(dir, 0700); e != nil {
			t.Fatal(e)
		}
	}
	binary := filepath.Join(base, "fake-docker")
	if e = os.WriteFile(binary, []byte("#!/bin/sh\n"+body), 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(ws, "fake-peer"), []byte("fake-peer-bytes"), 0500); e != nil {
		t.Fatal(e)
	}
	options := DockerOptions{Binary: binary, Endpoint: "unix:///synthetic-socket", ConfigDir: config, Workspace: ws, Image: "sha256:" + strings.Repeat("a", 64), Executable: "fake-peer", Nonce: strings.Repeat("b", 32)}
	runtime, e := NewDocker(options)
	if e != nil {
		t.Fatal(e)
	}
	return options, runtime
}
func TestDockerSpecHasOnlyRegisteredMountAndNoCredentialInheritance(t *testing.T) {
	options, runtime := dockerFixture(t, "exit 0\n")
	task := "tsk_" + strings.Repeat("c", 32)
	spec, dir, env, e := runtime.Spec(task, []string{"app-server", "--listen", "stdio://"})
	if e != nil {
		t.Fatal(e)
	}
	joined := strings.Join(spec.Args, " ")
	for _, value := range []string{"--network=none", "--read-only", "--user=65532:65532", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pull=never", "dst=/workspace,readonly"} {
		if !strings.Contains(joined, value) {
			t.Fatal(value)
		}
	}
	if strings.Count(joined, "--mount") != 1 || strings.Contains(joined, "--privileged") || strings.Contains(joined, "--pid=host") || dir != options.ConfigDir || len(env) != 4 {
		t.Fatal(spec, dir, env)
	}
	if !runtime.Acquire() {
		t.Fatal("first task refused")
	}
	if runtime.Acquire() {
		t.Fatal("concurrent task accepted")
	}
	runtime.Release()
	if _, _, _, e = runtime.Spec("external-container", nil); e == nil {
		t.Fatal("external process selector")
	}
	if e = os.Rename(options.Workspace, options.Workspace+"-original"); e != nil {
		t.Fatal(e)
	}
	os.Mkdir(options.Workspace, 0700)
	os.WriteFile(filepath.Join(options.Workspace, "fake-peer"), []byte("fake-peer-bytes"), 0500)
	if _, _, _, e = runtime.Spec(task, nil); e == nil {
		t.Fatal("directory replacement accepted")
	}
}
func TestDockerRegistrationAndPayloadAttacks(t *testing.T) {
	options, _ := dockerFixture(t, "exit 0\n")
	for _, attack := range []string{"remote", "image", "executable", "credentials", "overlap"} {
		o := options
		switch attack {
		case "remote":
			o.Endpoint = "tcp://127.0.0.1:2375"
		case "image":
			o.Image = "alpine:latest"
		case "executable":
			o.Executable = "../private"
		case "credentials":
			os.WriteFile(filepath.Join(o.ConfigDir, "config.json"), []byte("FAKE_CREDENTIAL_CONFIG"), 0600)
		case "overlap":
			o.ConfigDir = o.Workspace
		}
		if _, e := NewDocker(o); e == nil {
			t.Fatal(attack)
		}
		os.Remove(filepath.Join(options.ConfigDir, "config.json"))
	}
	_, runtime := dockerFixture(t, "exit 0\n")
	os.Chmod(filepath.Join(runtime.workspace, runtime.executable), 0700)
	os.WriteFile(filepath.Join(runtime.workspace, runtime.executable), []byte("changed"), 0500)
	if _, _, _, e := runtime.Spec("tsk_"+strings.Repeat("d", 32), nil); e == nil {
		t.Fatal("payload drift accepted")
	}
}
func TestDockerStopRequiresDaemonEvidenceAndBoundsOutput(t *testing.T) {
	task := "tsk_" + strings.Repeat("d", 32)
	_, ok := dockerFixture(t, "exit 0\n")
	if !ok.StopConfirmed(task) {
		t.Fatal("empty daemon result should confirm absence")
	}
	_, down := dockerFixture(t, "exit 1\n")
	if down.StopConfirmed(task) {
		t.Fatal("client exit confused with daemon workload stop")
	}
	_, hostile := dockerFixture(t, "printf 'not-an-owned-container\\n'\n")
	if hostile.StopConfirmed(task) {
		t.Fatal("unvalidated container ID accepted")
	}
	var output controlBuffer
	output.Write([]byte(strings.Repeat("x", 8192)))
	if !output.truncated || output.Len() != 4096 {
		t.Fatal("unbounded capture")
	}
}

func TestDockerMulticallInvocationAndRetarget(t *testing.T) {
	options, _ := dockerFixture(t, "exit 0\n")
	entry := filepath.Join(filepath.Dir(options.Binary), "docker")
	if e := os.Symlink(options.Binary, entry); e != nil {
		t.Fatal(e)
	}
	options.Binary = entry
	runtime, e := NewDocker(options)
	if e != nil {
		t.Fatal(e)
	}
	task := "tsk_" + strings.Repeat("c", 32)
	spec, _, _, e := runtime.Spec(task, nil)
	if e != nil || spec.Command != entry {
		t.Fatal("multicall entry lost", spec, e)
	}
	os.Remove(entry)
	os.Symlink("/bin/sh", entry)
	if _, _, _, e = runtime.Spec(task, nil); e == nil {
		t.Fatal("binary retarget accepted")
	}
	if runtime.StopConfirmed(task) {
		t.Fatal("retargeted control binary used")
	}
}

func TestDockerConfigCannotBeMounted(t *testing.T) {
	options, _ := dockerFixture(t, "exit 0\n")
	options.ConfigDir = filepath.Join(options.Workspace, "config")
	if e := os.Mkdir(options.ConfigDir, 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := NewDocker(options); e == nil {
		t.Fatal("control config mounted into tool filesystem")
	}
}
