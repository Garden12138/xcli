//go:build darwin || linux

package agentartifact

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"

	api "github.com/Garden12138/xcli/internal/agentapi"
)

func picture() []byte {
	var b bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 3, 2))
	im.Set(1, 1, color.RGBA{R: 255, A: 255})
	_ = png.Encode(&b, im)
	return b.Bytes()
}
func TestSafeFileImportAndOriginalBytes(t *testing.T) {
	root := t.TempDir()
	if e := os.Mkdir(filepath.Join(root, "task"), 0o700); e != nil {
		t.Fatal(e)
	}
	data := picture()
	if e := os.WriteFile(filepath.Join(root, "task", "output.jpeg"), data, 0o600); e != nil {
		t.Fatal(e)
	}
	actual, e := ReadBound(root, "task/output.jpeg", 20<<20)
	if e != nil {
		t.Fatal(e)
	}
	info, e := Validate(actual, "")
	if e != nil || info.MIME != "image/png" || info.Width != 3 || info.Height != 2 {
		t.Fatal(info, e)
	}
	if _, e = Validate(actual, "image/jpeg"); e == nil {
		t.Fatal("fake MIME accepted")
	}
	if _, e = Validate([]byte("image generated successfully"), ""); e == nil {
		t.Fatal("text accepted as image")
	}
	id := api.ID("art")
	if e = Put(root, "artifacts", id, actual); e != nil {
		t.Fatal(e)
	}
	copy, e := Read(root, "artifacts", id, 20<<20)
	if e != nil || !bytes.Equal(data, copy) {
		t.Fatal(e)
	}
	if e = Put(root, "artifacts", id, actual); e == nil {
		t.Fatal("overwrote immutable object")
	}
}
func TestEveryPathComponentAndHardlinkRejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "canary"), picture(), 0o600)
	_ = os.Mkdir(filepath.Join(root, "task"), 0o700)
	_ = os.Symlink(outside, filepath.Join(root, "linked"))
	_ = os.Symlink(filepath.Join(outside, "canary"), filepath.Join(root, "task", "linked"))
	_ = os.Link(filepath.Join(outside, "canary"), filepath.Join(root, "task", "hard"))
	for _, path := range []string{"linked/canary", "task/linked", "task/hard", "../canary", "/etc/passwd", "https://invalid/image.png", "task/../linked/canary"} {
		if _, e := ReadBound(root, path, 20<<20); e == nil {
			t.Fatal("unsafe import", path)
		}
	}
}
func TestReplacementRaceNeverReadsSymlinkCanary(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "task")
	_ = os.Mkdir(dir, 0o700)
	outside := filepath.Join(t.TempDir(), "canary")
	canary := []byte("private canary that must never be read")
	_ = os.WriteFile(outside, canary, 0o600)
	target := filepath.Join(dir, "candidate")
	data := picture()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 100 {
			temp := filepath.Join(dir, "new")
			_ = os.WriteFile(temp, data, 0o600)
			_ = os.Rename(temp, target)
			_ = os.Remove(target)
			_ = os.Symlink(outside, target)
			_ = os.Remove(target)
		}
	}()
	for range 100 {
		b, e := ReadBound(root, "task/candidate", 20<<20)
		if e == nil && !bytes.Equal(b, data) {
			t.Fatalf("unbound bytes imported: %q", b)
		}
	}
	wg.Wait()
}
