package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/engine"
	"github.com/Garden12138/xcli/internal/testutil"
)

func TestServeVersionAndShutdown(t *testing.T) {
	commandHomeScript(t, testutil.FakeScript{"codex": {{SleepMS: 10000, Stdout: []string{`{"type":"turn.completed"}`}}}})
	old := Version
	Version = "2.1.3-test"
	t.Cleanup(func() { Version = old })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := newRootCommand()
	root.SetContext(ctx)
	root.SetArgs([]string{"serve", "--listen", "127.0.0.1:0"})
	root.SetOut(io.Discard)
	reader, writer := io.Pipe()
	root.SetErr(writer)
	done := make(chan error, 1)
	go func() { done <- root.Execute(); writer.Close() }()
	defer reader.Close()
	var address, tokenFile string
	scan := bufio.NewScanner(reader)
	for scan.Scan() {
		line := scan.Text()
		if strings.HasPrefix(line, "监听：") {
			address = strings.TrimPrefix(line, "监听：")
		}
		if strings.HasPrefix(line, "令牌文件：") {
			tokenFile = strings.TrimPrefix(line, "令牌文件：")
			break
		}
	}
	if address == "" || tokenFile == "" {
		cancel()
		t.Fatalf("serve did not start: %v", scan.Err())
	}
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Get(address + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	var health struct {
		Version string `json:"version"`
	}
	err = json.NewDecoder(res.Body).Decode(&health)
	res.Body.Close()
	if err != nil || health.Version != Version {
		t.Fatalf("health: %+v %v", health, err)
	}
	token, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("POST", address+"/v1/tasks", strings.NewReader(`{"capability":"ask","agent":"codex","goal":"shutdown"}`))
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var submitted struct {
		ID string `json:"task_id"`
	}
	err = json.NewDecoder(res.Body).Decode(&submitted)
	res.Body.Close()
	if err != nil || res.StatusCode != 202 {
		t.Fatalf("submit: %+v %v", submitted, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(testutil.FakeCalls(t)) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if len(testutil.FakeCalls(t)) != 1 {
		t.Fatal("fake task did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not shut down")
	}
	e, err := engine.New(defaultHome())
	if err != nil {
		t.Fatal(err)
	}
	record, err := e.Status(submitted.ID)
	if err != nil || record.Status != api.StatusCancelled || record.Result == nil {
		t.Fatalf("shutdown result: %+v %v", record, err)
	}
	if _, _, err := runCommand([]string{"serve", "--help"}, ""); err != nil {
		t.Fatal(err)
	}
}
