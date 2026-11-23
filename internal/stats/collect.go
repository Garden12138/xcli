package stats

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/store"
)

type Sources struct {
	LedgerRecords    int      `json:"ledger_records"`
	BackfilledTasks  int      `json:"backfilled_tasks"`
	ApproximateTasks int      `json:"approximate_tasks"`
	SkippedLines     int      `json:"skipped_lines"`
	Homes            []string `json:"homes"`
}

func Homes(home string, include []string) []string {
	home = filepath.Clean(home)
	if abs, err := filepath.Abs(home); err == nil {
		home = abs
	}
	homes := []string{home}
	seen := map[string]bool{home: true}
	for _, pattern := range include {
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(home, pattern)
		}
		paths, _ := filepath.Glob(pattern)
		for _, path := range paths {
			path = filepath.Clean(path)
			if info, err := os.Stat(path); err == nil && info.IsDir() && !seen[path] {
				seen[path] = true
				homes = append(homes, path)
			}
		}
	}
	return homes
}

func Collect(homes []string) ([]Record, Sources, error) {
	records := []Record{}
	sources := Sources{Homes: make([]string, 0, len(homes))}
	seen := map[string]bool{}
	userHome, _ := os.UserHomeDir()
	// Load all ledgers first so a ledger entry takes priority in every data directory.
	for _, home := range homes {
		sources.Homes = append(sources.Homes, displayHome(home, userHome))
		rows, skipped, err := ReadLedger(ledgerPath(home))
		if err != nil {
			return nil, sources, fmt.Errorf("请检查统计账本的读取权限：%w", err)
		}
		sources.SkippedLines += skipped
		for _, record := range rows {
			if !seen[record.TaskID] {
				seen[record.TaskID] = true
				sources.LedgerRecords++
				records = append(records, record)
			}
		}
	}
	for _, home := range homes {
		rows, err := backfill(home, seen)
		if err != nil {
			return nil, sources, err
		}
		sources.BackfilledTasks += len(rows)
		records = append(records, rows...)
	}
	for _, record := range records {
		if record.Approximate {
			sources.ApproximateTasks++
		}
	}
	return records, sources, nil
}

func displayHome(path, userHome string) string {
	if userHome != "" {
		if path == userHome {
			return "~"
		}
		if rel, ok := strings.CutPrefix(path, userHome+string(filepath.Separator)); ok {
			return filepath.Join("~", rel)
		}
	}
	return path
}

func backfill(home string, seen map[string]bool) ([]Record, error) {
	tasks := filepath.Join(home, "tasks")
	dirs, err := os.ReadDir(tasks)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("请检查任务目录的读取权限：%w", err)
	}
	var records []Record
	for _, entry := range dirs {
		if !entry.IsDir() || seen[entry.Name()] {
			continue
		}
		dir := filepath.Join(tasks, entry.Name())
		rec, err := store.ReadRecord(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !rec.Status.Finished() || rec.Status == api.StatusInvalid || rec.Result == nil {
			continue
		}
		events, err := store.ReadEvents(dir, 0)
		if err != nil {
			return nil, fmt.Errorf("请检查任务 %s 的 events.jsonl：%w", entry.Name(), err)
		}
		if record, ok := FromTask(rec, events, nil); ok {
			seen[record.TaskID] = true
			records = append(records, record)
		}
	}
	return records, nil
}
