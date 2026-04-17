package main

import (
	"fmt"
	"os"

	"gdbtools/internal/mysqlcompare"
)

func main() {
	exitCode, err := mysqlcompare.Run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(exitCode)
}
