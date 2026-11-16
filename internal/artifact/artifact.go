package artifact

import (
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	_ "golang.org/x/image/webp"
)

type CollectOptions struct {
	OutDir    string
	Since     time.Time
	Text      string
	Files     []string
	ExtraDirs []string
	Prefix    string
}

func Inspect(path string) (api.Artifact, error) {
	file, err := os.Open(path)
	if err != nil {
		return api.Artifact{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return api.Artifact{}, fmt.Errorf("请选择普通图片文件：%s", path)
	}
	config, format, err := image.DecodeConfig(file)
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return api.Artifact{}, fmt.Errorf("请检查图片格式与尺寸：%s", path)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return api.Artifact{}, err
	}
	hash := sha256.New()
	bytes, err := io.Copy(hash, file)
	if err != nil {
		return api.Artifact{}, err
	}
	absolute, err := filepath.Abs(path)
	return api.Artifact{Path: absolute, Kind: "image", MIME: "image/" + format, Width: config.Width, Height: config.Height, Bytes: bytes, SHA256: fmt.Sprintf("%x", hash.Sum(nil))}, err
}

var imagePaths = regexp.MustCompile(`(/[^\s"'<>()]+\.(?i:png|jpe?g|webp|gif))`)

func scan(root string, since time.Time, depth, max int) []string {
	paths := []string{}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return paths
	}
	count := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		count++
		if max > 0 && count > max {
			return filepath.SkipAll
		}
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if depth > 0 && rel != "." && strings.Count(rel, string(os.PathSeparator)) >= depth-1 {
				return filepath.SkipDir
			}
			return nil
		}
		if info, err := d.Info(); err == nil && info.Mode().IsRegular() && !info.ModTime().Before(since) {
			paths = append(paths, path)
		}
		return nil
	})
	return paths
}

func copyUnique(source, target string) (string, error) {
	ext := filepath.Ext(target)
	base := strings.TrimSuffix(target, ext)
	for n := 0; ; n++ {
		path := target
		if n > 0 {
			path = fmt.Sprintf("%s-%d%s", base, n, ext)
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		input, err := os.Open(source)
		if err == nil {
			_, err = io.Copy(file, input)
			_ = input.Close()
		}
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			_ = os.Remove(path)
			return "", fmt.Errorf("请检查图片输出目录的写入权限：%v %v", err, closeErr)
		}
		return path, nil
	}
}

func CollectImages(o CollectOptions) ([]api.Artifact, error) {
	if err := os.MkdirAll(o.OutDir, 0700); err != nil {
		return nil, err
	}
	out, err := filepath.Abs(o.OutDir)
	if err != nil {
		return nil, err
	}
	canonicalOut, err := filepath.EvalSymlinks(out)
	if err != nil {
		return nil, err
	}
	candidates := scan(canonicalOut, o.Since, 0, 0)
	candidates = append(candidates, o.Files...)
	candidates = append(candidates, imagePaths.FindAllString(o.Text, -1)...)
	home, _ := os.UserHomeDir()
	for _, root := range o.ExtraDirs {
		if root == "~" {
			root = home
		} else if strings.HasPrefix(root, "~/") {
			root = filepath.Join(home, root[2:])
		}
		candidates = append(candidates, scan(root, o.Since, 3, 2000)...)
	}
	images := []api.Artifact{}
	seenPaths, seenHashes := map[string]bool{}, map[string]bool{}
	for _, source := range candidates {
		if !filepath.IsAbs(source) {
			continue
		}
		canonical, err := filepath.EvalSymlinks(source)
		if err != nil || seenPaths[canonical] {
			continue
		}
		seenPaths[canonical] = true
		image, err := Inspect(canonical)
		if err != nil || seenHashes[image.SHA256] {
			continue
		}
		rel, err := filepath.Rel(canonicalOut, canonical)
		inside := err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
		path := canonical
		if inside {
			path = filepath.Join(out, rel)
		}
		ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}[image.MIME]
		currentExt := strings.ToLower(filepath.Ext(path))
		if currentExt == ".jpeg" {
			currentExt = ".jpg"
		}
		if !inside || currentExt != ext {
			target := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) + ext
			if inside {
				target = filepath.Join(filepath.Dir(path), target)
			} else {
				if o.Prefix != "" {
					target = o.Prefix + "-" + target
				}
				target = filepath.Join(out, target)
			}
			placed, err := copyUnique(path, target)
			if err != nil {
				return nil, err
			}
			if inside {
				if err := os.Remove(path); err != nil {
					return nil, err
				}
			}
			path = placed
		}
		image, err = Inspect(path)
		if err != nil {
			return nil, err
		}
		seenHashes[image.SHA256] = true
		images = append(images, image)
	}
	return images, nil
}
