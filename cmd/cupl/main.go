package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	cuplroot "github.com/pborges/cupl"
	cupllang "github.com/pborges/cupl/internal/cupl"
	"github.com/pborges/cupl/internal/gal"
	"github.com/pborges/cupl/internal/jed"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "-v":
		fmt.Println(cuplroot.Version())
	case "build":
		if err := cmdBuild(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "devices":
		fmt.Println("g16v8as")
		fmt.Println("g22v10")
	case "version":
		fmt.Println(cuplroot.Version())
	case "burn":
		if err := cmdBurn(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
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
	fmt.Println("  cupl burn <file.jed|file.pld>")
	fmt.Println("  cupl devices")
	fmt.Println("  cupl version")
	fmt.Println("  cupl -v")
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
	content, err := cupllang.Parse(data)
	if err != nil {
		return err
	}
	g, err := cupllang.Compile(content)
	if err != nil {
		return err
	}
	if outPath == "" {
		base := strings.TrimSuffix(inPath, filepath.Ext(inPath))
		outPath = base + ".jed"
	}
	return buildJedFromContent(content, g, outPath)
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

func buildJed(inPath, outPath string) error {
	data, err := ioutil.ReadFile(inPath)
	if err != nil {
		return err
	}
	content, err := cupllang.Parse(data)
	if err != nil {
		return err
	}
	g, err := cupllang.Compile(content)
	if err != nil {
		return err
	}
	return buildJedFromContent(content, g, outPath)
}

func buildJedFromContent(content cupllang.Content, g *gal.GAL, outPath string) error {
	jedText := jed.MakeJEDEC(jed.Config{
		SecurityBit: false,
		Header:      headerLines(content, g.Chip),
	}, g)
	return ioutil.WriteFile(outPath, []byte(jedText), 0644)
}

func cmdBurn(args []string) error {
	deviceOverride, rest, err := parseBurnArgs(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return errors.New("burn requires a single .jed or .pld input")
	}
	inPath := rest[0]
	ext := strings.ToLower(filepath.Ext(inPath))
	jedPath := inPath
	tempDir := ""
	if ext == ".pld" {
		tempDir, err = os.MkdirTemp("", "cupl-burn-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)
		base := strings.TrimSuffix(filepath.Base(inPath), filepath.Ext(inPath))
		jedPath = filepath.Join(tempDir, base+".jed")
		if err := buildJed(inPath, jedPath); err != nil {
			return err
		}
	} else if ext != ".jed" {
		return errors.New("burn requires a .jed or .pld input")
	}
	data, err := ioutil.ReadFile(jedPath)
	if err != nil {
		return err
	}
	device := deviceOverride
	if device == "" {
		device, err = jedDeviceFromFile(data)
		if err != nil {
			return err
		}
	}
	cmd := exec.Command("minipro", "-p", device, "-w", jedPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func parseBurnArgs(args []string) (string, []string, error) {
	fs := flag.NewFlagSet("burn", flag.ContinueOnError)
	device := fs.String("p", "", "minipro device name (override)")
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-p" || arg == "--p" || arg == "--device" {
			if i+1 >= len(args) {
				return "", nil, errors.New("missing value for -p")
			}
			if err := fs.Set("p", args[i+1]); err != nil {
				return "", nil, err
			}
			i++
			continue
		}
		if strings.HasPrefix(arg, "-p=") {
			if err := fs.Set("p", strings.TrimPrefix(arg, "-p=")); err != nil {
				return "", nil, err
			}
			continue
		}
		if strings.HasPrefix(arg, "--device=") {
			if err := fs.Set("p", strings.TrimPrefix(arg, "--device=")); err != nil {
				return "", nil, err
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if err := fs.Parse([]string{arg}); err != nil {
				return "", nil, err
			}
			continue
		}
		rest = append(rest, arg)
	}
	return *device, rest, nil
}

func jedDeviceFromFile(data []byte) (string, error) {
	s := string(data)
	s = strings.TrimPrefix(s, "\x02")
	if idx := strings.Index(s, "\x03"); idx >= 0 {
		s = s[:idx]
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "*") {
			break
		}
		if strings.HasPrefix(line, "Device") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "Device"))
			if v == "" {
				return "", errors.New("JED device header is empty")
			}
			fields := strings.Fields(v)
			if len(fields) == 0 {
				return "", errors.New("JED device header is empty")
			}
			return fields[0], nil
		}
	}
	return "", errors.New("JED device header not found")
}


func headerLines(c cupllang.Content, chip gal.Chip) []string {
	lines := []string{
		fmt.Sprintf("CUPlang        %s", cuplroot.Version()),
		fmt.Sprintf("Device          %s", headerDeviceName(chip)),
	}
	keys := []string{"Name", "Partno", "Revision", "Date", "Designer", "Company", "Assembly", "Location"}
	for _, k := range keys {
		if v := strings.TrimSpace(c.Meta[k]); v != "" {
			lines = append(lines, fmt.Sprintf("%-15s %s", k, v))
		}
	}
	return lines
}

func headerDeviceName(chip gal.Chip) string {
	return strings.ToLower(strings.TrimPrefix(chip.Name(), "GAL"))
}
