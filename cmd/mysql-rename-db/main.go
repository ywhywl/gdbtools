package main

import (
	"fmt"
	"os"

	"github.com/ywhywl/gdbtools/internal/mysqlrenamedb"
)

func main() {
	exitCode, err := mysqlrenamedb.Run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	os.Exit(exitCode)
}
