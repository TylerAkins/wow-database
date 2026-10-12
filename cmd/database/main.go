package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TylerAkins/wow-database/internal/database"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "database:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("command required: validate, compile, compare")
	}
	switch args[0] {
	case "validate":
		flags := flag.NewFlagSet("validate", flag.ContinueOnError)
		root := flags.String("root", "data", "directory containing canonical source datasets")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		rows, err := database.ValidateTree(*root)
		if err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
		return nil
	case "compile":
		flags := flag.NewFlagSet("compile", flag.ContinueOnError)
		flavor := flags.String("flavor", "", "game flavor: era or forever")
		sources := flags.String("sources", "", "source snapshot directory")
		out := flags.String("out", "", "compilation output directory")
		policy := flags.String("policy", "schemas/field-resolution.json", "field-level selection policy")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *flavor == "" {
			return fmt.Errorf("--flavor is required")
		}
		if *sources == "" {
			*sources = filepath.Join("data", *flavor, "sources")
		}
		if *out == "" {
			*out = filepath.Join("data", *flavor, "compilation")
		}
		return database.Compile(*flavor, *sources, *out, *policy)
	case "compare":
		flags := flag.NewFlagSet("compare", flag.ContinueOnError)
		left := flags.String("left", "", "left source or compilation dataset directory")
		right := flags.String("right", "", "right source or compilation dataset directory")
		out := flags.String("out", "", "comparison report directory")
		crossFlavor := flags.Bool("cross-flavor", false, "match entity type and ID across Era and Forever")
		coordinateTolerance := flags.Float64("coordinate-tolerance", 0, "ignore coordinate differences within this distance when map/zone matches")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *left == "" || *right == "" || *out == "" {
			return fmt.Errorf("--left, --right, and --out are required")
		}
		comparison, err := database.CompareWithTolerance(*left, *right, *crossFlavor, *coordinateTolerance)
		if err != nil {
			return err
		}
		return database.WriteComparison(*out, comparison)
	default:
		return fmt.Errorf("unknown command %q; expected validate, compile, compare", args[0])
	}
}
