//go:generate esc -o temlates.go templates
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	appName   = filepath.Base(os.Args[0])
	help      = flag.Bool("help", false, "show usage")
	typeNames = flag.String("type", "", "comma-separated list of type names; must be set")
	suffix    = flag.String("suffix", "kit", "output file suffix in <type>_<suffix>.go")
	output    = flag.String("output", "", "output file name (default \"<src dir>/<type>_<suffix>.go\")")
)

// Usage is a replacement usage function for the flags package.
func Usage() {
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  %s [flags] -type T [directory]\n", appName)
	fmt.Fprintf(os.Stderr, "  %s [flags[ -type T files... # Must be a single package\n", appName)
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
}

func parseCmdLine() []string {
	flag.Usage = Usage
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}
	if len(*typeNames) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	// We accept either one directory or a list of files. Which do we have?
	if files := flag.Args(); len(files) == 0 {
		// Default: process whole package in current directory.
		return []string{"."}
	} else {
		return files
	}
}

// isDirectory reports whether the named file is a directory.
func isDirectory(name string) bool {
	info, err := os.Stat(name)
	if err != nil {
		log.Fatal(err)
	}
	return info.IsDir()
}

func main() {
	log.SetFlags(0)
	log.SetPrefix(appName + ": ")

	files := parseCmdLine()

	types := strings.Split(*typeNames, ",")

	var g Generator

	if err := g.parse(files); err != nil {
		log.Fatalf("writing output: %s", err)
	}

	g.generateHeader()

	// Run generate for each type.
	for _, typeName := range types {
		if err := g.generateType(typeName); err != nil {
			log.Fatal("generating type: %s", err)
		}
	}

	// Format the output.
	src := g.format()

	// Write to file.
	if outputName := *output; outputName == "-" {
		os.Stdout.WriteString(string(src))
	} else {
		if outputName == "" {
			baseName := fmt.Sprintf("%s_%s.go", types[0], *suffix)
			outputName = filepath.Join(g.dir, strings.ToLower(baseName))
		}

		if err := ioutil.WriteFile(outputName, src, 0644); err != nil {
			log.Fatalf("writing output: %s", err)
		}
	}
}
