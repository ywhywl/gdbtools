package main

import (
	"fmt"
	"os"

	"github.com/ywhywl/gdbtools/internal/mysqlpricheck"
)

func main() {
	exitCode, err := mysqlpricheck.Run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	os.Exit(exitCode)
}
