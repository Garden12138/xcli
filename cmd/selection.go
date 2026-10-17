package cmd

import "os"

// Historical selection metadata remains readable; new runs use explicit or
// configured default selection, never the retired prompt routing rules.
type agentSelection struct {
	Agent  string
	Source string
	Rule   string
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
