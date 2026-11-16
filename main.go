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

	var exit *cmd.ExitError
	if errors.As(err, &exit) {
		if exit.Message != "" {
			fmt.Fprintln(os.Stderr, "错误：", exit.Message)
		}
		os.Exit(exit.Code)
	}
	fmt.Fprintln(os.Stderr, "错误：", err)
	os.Exit(1)
}
