// Package main is the entry point for the vtpc command-line tool.
package main

import (
	"os"

	"github.com/Norgate-AV/vtpc/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
