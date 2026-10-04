package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/TylerAkins/wow-database/internal/compile"
)

func main() {
	dumpPath := flag.String("dump", "", "JSONL dump from tools/export_questiedb.lua")
	outDir := flag.String("out", "export/forever", "published database directory")
	check := flag.Bool("check", false, "compare the dump with the published tree and write nothing")
	flag.Parse()
	if *dumpPath == "" {
		fmt.Fprintln(os.Stderr, "compile: --dump is required")
		os.Exit(2)
	}
	file, err := os.Open(*dumpPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "compile: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()
	err = compile.Run(compile.Options{Dump: file, OutDir: *outDir, Check: *check})
	if err != nil {
		fmt.Fprintf(os.Stderr, "compile: %v\n", err)
		os.Exit(1)
	}
}
