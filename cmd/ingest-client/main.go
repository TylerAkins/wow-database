package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/TylerAkins/wow-database/internal/clientimport"
)

func main() {
	flags := flag.NewFlagSet("ingest-client", flag.ExitOnError)
	input := flags.String("input", ".local/foreverclient", "local client-extract output directory")
	output := flags.String("out", "data/forever/sources/foreverclient", "canonical client dataset directory")
	if err := flags.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, errors.New("unexpected positional arguments"))
		os.Exit(2)
	}
	if err := clientimport.Import(*input, *output); err != nil {
		fmt.Fprintln(os.Stderr, "client-import:", err)
		os.Exit(1)
	}
	fmt.Printf("Imported local client data to %s\n", *output)
}
