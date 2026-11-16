//go:build unix

package proc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

func AcquireSlot(ctx context.Context, lockDir, name string, max int, onWait func()) (func(), error) {
	if max < 1 || name != filepath.Base(name) || name == "." || name == ".." {
		return nil, errors.New("请使用有效的 agent 名，并将 max_concurrency 设为正整数")
	}
	if err := os.MkdirAll(lockDir, 0700); err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for i := range max {
			file, err := os.OpenFile(filepath.Join(lockDir, fmt.Sprintf("%s-%d.lock", name, i)), os.O_CREATE|os.O_RDWR, 0600)
			if err != nil {
				return nil, err
			}
			if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
				var once sync.Once
				return func() { once.Do(func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }) }, nil
			}
			_ = file.Close()
			if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
				return nil, err
			}
		}
		if onWait != nil {
			onWait()
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
