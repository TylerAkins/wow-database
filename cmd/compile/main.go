package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/TylerAkins/wow-database/internal/att"
	"github.com/TylerAkins/wow-database/internal/compile"
)

func main() {
	dumpPath := flag.String("dump", "", "JSONL dump from tools/export_questiedb.lua")
	outDir := flag.String("out", "export/forever", "published database directory")
	check := flag.Bool("check", false, "compare the dump with the published tree and write nothing")
	attRoot := flag.String("att-root", "", "ATT checkout containing .contrib/.db/forever")
	attCommit := flag.String("att-commit", "", "ATT source commit SHA")
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
	var locations *att.Result
	if *attRoot != "" {
		result, readErr := att.Read(*attRoot)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "compile: %v\n", readErr)
			os.Exit(1)
		}
		locations = &result
	}
	err = compile.Run(compile.Options{Dump: file, OutDir: *outDir, Check: *check, ATT: locations, ATTCommit: *attCommit})
	if err != nil {
		fmt.Fprintf(os.Stderr, "compile: %v\n", err)
		os.Exit(1)
	}
}
