package main

import (
	"fmt"
	"os"

	"github.com/ywhywl/gdbtools/internal/insightdbmgr"
)

func main() {
	exitCode, err := insightdbmgr.Run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(exitCode)
}
