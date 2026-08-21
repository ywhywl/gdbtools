package main

import (
	"fmt"
	"os"

	"github.com/ywhywl/gdbtools/internal/insightbatchcn"
)

func main() {
	exitCode, err := insightbatchcn.Run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(exitCode)
}
