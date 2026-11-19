//go:build unix

package proc

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"strings"
)

func proxyRedactor(env []string) *strings.Replacer {
	var values []string
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !isProxyKey(key) || value == "" {
			continue
		}
		encoded, _ := json.Marshal(value)
		values = append(values, string(encoded[1:len(encoded)-1]))
		for _, line := range strings.Split(value, "\n") {
			if line != "" {
				values = append(values, line)
			}
		}
	}
	if len(values) == 0 {
		return nil
	}
	sort.SliceStable(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	var replacements []string
	for _, value := range values {
		replacements = append(replacements, value, "[redacted]")
	}
	return strings.NewReplacer(replacements...)
}

// Buffer whole lines so values split across process writes can still be redacted.
type redactedLog struct {
	dst     io.Writer
	replace *strings.Replacer
	line    []byte
	dropped bool
}

func (w *redactedLog) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		part, rest, end := bytes.Cut(p, []byte{'\n'})
		if len(w.line)+len(part) > maxOutputLine {
			w.line, w.dropped = nil, true
		}
		if !w.dropped {
			w.line = append(w.line, part...)
		}
		if end {
			if err := w.flushLine(true); err != nil {
				return n - len(p), err
			}
		}
		p = rest
	}
	return n, nil
}

func (w *redactedLog) flush() error { return w.flushLine(false) }

func (w *redactedLog) flushLine(newline bool) error {
	line := string(w.line)
	if w.dropped {
		line = "[xcli] 已跳过超长日志行"
	}
	if newline {
		line += "\n"
	}
	w.line, w.dropped = w.line[:0], false
	_, err := w.replace.WriteString(w.dst, line)
	return err
}
