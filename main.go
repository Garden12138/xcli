package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Garden12138/xcli/cmd"
)

func main() {
	err := cmd.Execute()
	if err == nil {
		return
	}

	var exitErr *cmd.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.Message != "" {
			fmt.Fprintln(os.Stderr, exitErr.Message)
		}
		os.Exit(exitErr.Code)
	}

	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}
