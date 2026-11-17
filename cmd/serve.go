package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Garden12138/xcli/internal/engine"
	"github.com/Garden12138/xcli/internal/server"
	"github.com/spf13/cobra"
)

func newServeCommand() *cobra.Command {
	var listen string
	c := &cobra.Command{Use: "serve", Short: "启动本机 HTTP 任务接口", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		e, err := engine.New(rootOpts.home)
		if err != nil {
			return err
		}
		token, err := server.LoadOrCreateToken(e.Home)
		if err != nil {
			return err
		}
		listener, err := net.Listen("tcp", listen)
		if err != nil {
			return fmt.Errorf("请检查 --listen 地址及端口占用：%w", err)
		}
		defer listener.Close()
		server.Version = Version
		h := server.New(e, token)
		defer h.(io.Closer).Close()
		srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
		ctx, stop := taskContext(c.Context())
		defer stop()
		done := make(chan error, 1)
		go func() { done <- srv.Serve(listener) }()
		fmt.Fprintf(c.ErrOrStderr(), "监听：http://%s\n令牌文件：%s\n", listener.Addr(), filepath.Join(e.Home, "server.token"))
		select {
		case err := <-done:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
			_ = h.(io.Closer).Close()
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return srv.Shutdown(shutdown)
		}
	}}
	c.Flags().StringVar(&listen, "listen", "127.0.0.1:7788", "HTTP 监听地址")
	return c
}
