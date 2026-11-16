package main

import (
	"fmt"
	"os"

	"github.com/Garden12138/xcli/cmd"
)

func main() {
	err := cmd.Execute()
	if err == nil {
		return
	}

	fmt.Fprintln(os.Stderr, "错误：", err)
	os.Exit(1)
}
