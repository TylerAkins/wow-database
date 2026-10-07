package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/TylerAkins/wow-database/internal/compile"
)

func main() {
	dumpPath := flag.String("dump", "", "optional JSONL dump for tests; omit when using --att-root")
	outDir := flag.String("out", "export/forever", "published database directory")
	check := flag.Bool("check", false, "compare the dump with the published tree and write nothing")
	attRoot := flag.String("att-root", "", "ATT checkout containing .contrib/.db/forever")
	attCommit := flag.String("att-commit", "", "ATT source commit SHA")
	generatedAt := flag.String("generated-at", "", "export timestamp (RFC3339); required with --att-root")
	flag.Parse()

	var dump *os.File
	if *dumpPath != "" {
		file, err := os.Open(*dumpPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "compile: %v\n", err)
			os.Exit(1)
		}
		dump = file
		defer dump.Close()
	}
	if dump == nil && *attRoot == "" {
		fmt.Fprintln(os.Stderr, "compile: --att-root is required unless --dump is set")
		os.Exit(2)
	}
	err := compile.Run(compile.Options{
		Dump:        dump,
		ATTRoot:     *attRoot,
		ATTCommit:   *attCommit,
		GeneratedAt: *generatedAt,
		OutDir:      *outDir,
		Check:       *check,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "compile: %v\n", err)
		os.Exit(1)
	}
}
