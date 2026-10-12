package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/TylerAkins/wow-database/internal/clientextract"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "client-extract:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("client-extract", flag.ContinueOnError)
	flags.SetOutput(stderr)

	clientDir := flags.String("client-dir", "", "WoW install directory (auto-detected on macOS when omitted)")
	product := flags.String("product", "", "product id from .build.info, for example wow_classic")
	dbdDir := flags.String("dbd-dir", "", "local WoWDBDefs checkout containing manifest.json and definitions/")
	outDir := flags.String("out", ".local/foreverclient", "local output directory; never inside the game install")
	var tables stringList
	flags.Var(&tables, "table", "table name to extract; repeat to select several (default: every DB2 table in the local WoWDBDefs manifest)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if *dbdDir == "" {
		return errors.New("--dbd-dir is required; the extractor does not download schema data")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	report, err := clientextract.Extract(ctx, clientextract.Options{
		ClientDir: *clientDir,
		Product:   *product,
		DBDDir:    *dbdDir,
		OutputDir: *outDir,
		Tables:    tables,
	}, stdout)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Client build %s (%s): %d tables written, %d partial, %d unavailable\n",
		report.Client.Version, report.Client.Product, report.Counts.Written, report.Counts.Partial, report.Counts.Unavailable)
	return nil
}
