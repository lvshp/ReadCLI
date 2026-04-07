package main

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/lvshp/ReadCLI/core"
	"github.com/lvshp/ReadCLI/lib"
)

var Version string

func main() {
	lines := flag.Int("n", 0, "number of lines to display at once")
	showVersion := flag.Bool("v", false, "display version")
	flag.Parse()

	if *showVersion {
		lib.DisplayVersion(Version)
		os.Exit(0)
	}

	if *lines < 0 {
		println("line count must be greater than 0")
		os.Exit(1)
	}

	initialFile := ""
	if flag.NArg() > 0 {
		initialFile = flag.Arg(0)
		if abs, err := filepath.Abs(initialFile); err == nil {
			initialFile = abs
		}
	}

	core.Run(initialFile, *lines, Version)
}
