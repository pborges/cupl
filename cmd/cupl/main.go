package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pborges/cupl/internal/cupl"
	"github.com/pborges/cupl/internal/gal"
	"github.com/pborges/cupl/internal/jed"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "build":
		if err := cmdBuild(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "devices":
		fmt.Println("g16v8as")
		fmt.Println("g22v10")
	case "version":
		fmt.Println(version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println("cupl - WinCUPL-compatible compiler")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  cupl build <file.pld> -o <file.jed>")
	fmt.Println("  cupl devices")
	fmt.Println("  cupl version")
}

func cmdBuild(args []string) error {
	outPath, rest, err := parseBuildArgs(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return errors.New("build requires a single .pld input")
	}
	inPath := rest[0]
	data, err := ioutil.ReadFile(inPath)
	if err != nil {
		return err
	}
	content, err := cupl.Parse(data)
	if err != nil {
		return err
	}
	g, err := cupl.Compile(content)
	if err != nil {
		return err
	}
	if outPath == "" {
		base := strings.TrimSuffix(inPath, filepath.Ext(inPath))
		outPath = base + ".jed"
	}
	jedText := jed.MakeJEDEC(jed.Config{
		SecurityBit: false,
		Header:      headerLines(content, g.Chip),
	}, g)
	return ioutil.WriteFile(outPath, []byte(jedText), 0644)
}

func parseBuildArgs(args []string) (string, []string, error) {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	outPath := fs.String("o", "", "output JED file")
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-o" || arg == "--o" {
			if i+1 >= len(args) {
				return "", nil, errors.New("missing value for -o")
			}
			if err := fs.Set("o", args[i+1]); err != nil {
				return "", nil, err
			}
			i++
			continue
		}
		if strings.HasPrefix(arg, "-o=") {
			if err := fs.Set("o", strings.TrimPrefix(arg, "-o=")); err != nil {
				return "", nil, err
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			// Let FlagSet handle known flags to preserve error messages.
			if err := fs.Parse([]string{arg}); err != nil {
				return "", nil, err
			}
			continue
		}
		rest = append(rest, arg)
	}
	return *outPath, rest, nil
}

func headerLines(c cupl.Content, chip gal.Chip) []string {
	lines := []string{
		fmt.Sprintf("CUPlang        %s", version),
		fmt.Sprintf("Device          %s", strings.ToLower(strings.TrimPrefix(chip.Name(), "GAL"))),
	}
	keys := []string{"Name", "Partno", "Revision", "Date", "Designer", "Company", "Assembly", "Location"}
	for _, k := range keys {
		if v := strings.TrimSpace(c.Meta[k]); v != "" {
			lines = append(lines, fmt.Sprintf("%-15s %s", k, v))
		}
	}
	return lines
}
