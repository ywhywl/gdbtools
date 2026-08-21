package main

import (
	"fmt"
	"os"

	"github.com/ywhywl/gdbtools/internal/mysqldbmanager"
)

func main() {
	exitCode, err := mysqldbmanager.Run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	os.Exit(exitCode)
}
