package isolatedruntime

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWholeProcessSpecRefusesBroadOrReplacedRoots(t *testing.T) {
	base, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(base, 0700); e != nil {
		t.Fatal(e)
	}
	home, workspace, binary := filepath.Join(base, "home"), filepath.Join(base, "workspace"), filepath.Join(base, "selected-cli")
	for _, dir := range []string{home, workspace} {
		if e = os.Mkdir(dir, 0700); e != nil {
			t.Fatal(e)
		}
	}
	if e = os.WriteFile(binary, []byte("fixture only"), 0700); e != nil {
		t.Fatal(e)
	}
	spec, e := Seatbelt(binary, workspace, home, []string{"app-server", "--listen", "stdio://"})
	if runtime.GOOS != "darwin" {
		if e == nil {
			t.Fatal("unsupported platform promoted")
		}
		return
	}
	if e != nil {
		t.Fatal(e)
	}
	if spec.Command != "/usr/bin/sandbox-exec" || !strings.Contains(spec.Args[1], "(deny default)") || strings.Contains(spec.Args[1], "allow network") || strings.Contains(spec.Args[1], "allow mach-lookup") {
		t.Fatal(spec)
	}
	if _, e = Seatbelt(binary, "/", home, nil); e == nil {
		t.Fatal("root grant")
	}
	if e = os.Chmod(home, 0755); e != nil {
		t.Fatal(e)
	}
	if _, e = Seatbelt(binary, workspace, home, nil); e == nil {
		t.Fatal("public home grant")
	}
	os.Chmod(home, 0700)
	os.Rename(workspace, workspace+"-moved")
	os.Symlink(workspace+"-moved", workspace)
	if _, e = Seatbelt(binary, workspace, home, nil); e == nil {
		t.Fatal("symlink root grant")
	}
}
