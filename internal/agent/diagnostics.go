package agent

import "strings"

// StderrDiagnostics recognizes explicit native failures that some CLI versions
// report only on stderr despite returning a successful protocol result.
// It is written by the stderr copier and read only after the process has exited.
type StderrDiagnostics struct {
	adapter          string
	window           string
	permissionDenied bool
}

func NewStderrDiagnostics(adapter string) *StderrDiagnostics {
	return &StderrDiagnostics{adapter: adapter}
}

func (d *StderrDiagnostics) Write(data []byte) (int, error) {
	n := len(data)
	if d.adapter != "antigravity" || d.permissionDenied {
		return n, nil
	}
	// Scan bounded overlapping windows, including when a single write contains
	// megabytes of diagnostics. A later stderr flood cannot erase the finding.
	for len(data) > 0 {
		size := min(len(data), 1024)
		d.window += string(data[:size])
		data = data[size:]
		for _, line := range strings.Split(d.window, "\n") {
			if strings.Contains(line, "jetski: no output produced") &&
				strings.Contains(line, "headless mode cannot prompt") &&
				strings.Contains(line, "auto-denied") {
				d.permissionDenied = true
				d.window = ""
				return n, nil
			}
		}
		if len(d.window) > 4096 {
			d.window = d.window[len(d.window)-4096:]
		}
	}
	return n, nil
}

func (d *StderrDiagnostics) Apply(result RunResult) RunResult {
	if d.permissionDenied && result.Output == "" && result.Status == "success" {
		result.Status = "failed"
		result.ErrorKind = "process_failed"
		result.Error = "antigravity: headless tool permission was auto-denied; configure a targeted native permissions.allow rule before retrying"
	}
	return result
}
