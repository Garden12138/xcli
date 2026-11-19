package artifact

import (
	"crypto/sha256"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeImage(t *testing.T, path, format string, width int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, 10))
	if format == "jpeg" {
		err = jpeg.Encode(file, img, nil)
	} else {
		err = png.Encode(file, img)
	}
	if closeErr := file.Close(); err != nil || closeErr != nil {
		t.Fatalf("image write: %v, %v", err, closeErr)
	}
}

func TestInspectPNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.png")
	writeImage(t, path, "png", 12)
	a, err := Inspect(path)
	raw, _ := os.ReadFile(path)
	if err != nil || a.MIME != "image/png" || a.Width != 12 || a.Height != 10 || a.SHA256 != fmt.Sprintf("%x", sha256.Sum256(raw)) || a.Bytes != int64(len(raw)) || a.Kind != "image" {
		t.Fatalf("inspect: %+v, %v", a, err)
	}
}

func TestCollectFromOutDir(t *testing.T) {
	out := t.TempDir()
	old := filepath.Join(out, "old.png")
	writeImage(t, old, "png", 9)
	before := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, before, before); err != nil {
		t.Fatal(err)
	}
	since := time.Now()
	writeImage(t, filepath.Join(out, "new.png"), "png", 8)
	if err := os.WriteFile(filepath.Join(out, "fake.png"), []byte("fake"), 0600); err != nil {
		t.Fatal(err)
	}
	images, err := CollectImages(CollectOptions{OutDir: out, Since: since})
	if err != nil || len(images) != 1 || filepath.Base(images[0].Path) != "new.png" {
		t.Fatalf("collected: %+v, %v", images, err)
	}
}

func TestCollectFixesExtension(t *testing.T) {
	out := t.TempDir()
	path := filepath.Join(out, "a.png")
	writeImage(t, path, "jpeg", 8)
	images, err := CollectImages(CollectOptions{OutDir: out})
	if err != nil || len(images) != 1 || filepath.Base(images[0].Path) != "a.jpg" || images[0].MIME != "image/jpeg" {
		t.Fatalf("fixed extension: %+v, %v", images, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("original wrong extension still present")
	}
}

func TestCollectCopiesExternal(t *testing.T) {
	out := t.TempDir()
	source := filepath.Join(t.TempDir(), "gen.png")
	writeImage(t, source, "png", 8)
	images, err := CollectImages(CollectOptions{OutDir: out, Prefix: "main", Text: "已保存到 " + source + "。"})
	if err != nil || len(images) != 1 || filepath.Base(images[0].Path) != "main-gen.png" {
		t.Fatalf("external: %+v, %v", images, err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("external original changed")
	}
}

func TestCollectExtraDirs(t *testing.T) {
	extra, out := t.TempDir(), t.TempDir()
	writeImage(t, filepath.Join(extra, "one", "allowed.png"), "png", 8)
	writeImage(t, filepath.Join(extra, "one", "two", "edge.png"), "png", 9)
	writeImage(t, filepath.Join(extra, "one", "two", "three", "deep.png"), "png", 10)
	images, err := CollectImages(CollectOptions{OutDir: out, ExtraDirs: []string{extra}, Prefix: "step"})
	if err != nil || len(images) != 2 {
		t.Fatalf("extra dir depth: %+v, %v", images, err)
	}
	for _, a := range images {
		if strings.Contains(a.Path, "deep") {
			t.Fatal("extra scanning exceeded three levels")
		}
	}
}

func TestCollectDedup(t *testing.T) {
	out := t.TempDir()
	path := filepath.Join(out, "image.png")
	writeImage(t, path, "png", 8)
	images, err := CollectImages(CollectOptions{OutDir: out, Files: []string{path, path}, Text: path})
	if err != nil || len(images) != 1 {
		t.Fatalf("dedup: %+v, %v", images, err)
	}
}

func TestCollectExtraDirsPrefersCurrentSession(t *testing.T) {
	out, cache := t.TempDir(), t.TempDir()
	writeImage(t, filepath.Join(cache, "other", "other.png"), "png", 8)
	writeImage(t, filepath.Join(cache, "current", "current.png"), "png", 9)
	images, err := CollectImages(CollectOptions{OutDir: out, ExtraDirs: []string{cache}, SessionID: "current"})
	if err != nil || len(images) != 1 || filepath.Base(images[0].Path) != "current.png" {
		t.Fatalf("current session: %+v, %v", images, err)
	}
}

func TestCollectPrefersOutDirOverNativeCache(t *testing.T) {
	out, cache := t.TempDir(), t.TempDir()
	writeImage(t, filepath.Join(out, "image.png"), "png", 8)
	writeImage(t, filepath.Join(cache, "native.png"), "png", 9)
	images, err := CollectImages(CollectOptions{OutDir: out, ExtraDirs: []string{cache}, Prefix: "main"})
	if err != nil || len(images) != 1 || filepath.Base(images[0].Path) != "image.png" {
		t.Fatalf("output should suppress native cache scan: %+v, %v", images, err)
	}
}

func TestCollectPrefersExplicitPathsOverNativeCache(t *testing.T) {
	for _, source := range []string{"files", "text"} {
		t.Run(source, func(t *testing.T) {
			out, cache := t.TempDir(), t.TempDir()
			path := filepath.Join(t.TempDir(), "reported.png")
			writeImage(t, path, "png", 8)
			writeImage(t, filepath.Join(cache, "unrelated.png"), "png", 9)
			o := CollectOptions{OutDir: out, ExtraDirs: []string{cache}}
			if source == "files" {
				o.Files = []string{path}
			} else {
				o.Text = path
			}
			images, err := CollectImages(o)
			if err != nil || len(images) != 1 || filepath.Base(images[0].Path) != "reported.png" {
				t.Fatalf("explicit image should suppress native cache scan: %+v, %v", images, err)
			}
		})
	}
}

func TestCollectPreservesExistingTarget(t *testing.T) {
	out := t.TempDir()
	existing := filepath.Join(out, "a.jpg")
	writeImage(t, existing, "jpeg", 12)
	before, _ := os.ReadFile(existing)
	writeImage(t, filepath.Join(out, "a.png"), "jpeg", 8)
	images, err := CollectImages(CollectOptions{OutDir: out})
	after, _ := os.ReadFile(existing)
	if err != nil || len(images) != 2 || string(before) != string(after) {
		t.Fatalf("target preservation: %+v, %v", images, err)
	}
}
