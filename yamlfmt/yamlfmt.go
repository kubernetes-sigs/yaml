/*
Copyright 2021 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kylelemons/godebug/diff"
	yaml "sigs.k8s.io/yaml/goyaml.v3"
	"sigs.k8s.io/yaml/kyaml"
)

const (
	fmtYAML  = "yaml"
	fmtKYAML = "kyaml"
)

func main() {
	fs := flag.NewFlagSet("yamlfmt", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: %s [<yaml-files>...]\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(fs.Output(), "If no files are specified, stdin will be used.\n")
		fs.PrintDefaults()
	}

	diff := fs.Bool("d", false, "diff input files with their formatted versions")
	help := fs.Bool("h", false, "print help and exit")
	format := fs.String("o", "yaml", "output format: may be 'yaml' or 'kyaml'")
	write := fs.Bool("w", false, "write result to input files instead of stdout")
	fs.Parse(os.Args[1:])

	if *help {
		fs.SetOutput(os.Stdout)
		fs.Usage()
		os.Exit(0)
	}

	switch *format {
	case "yaml", "kyaml":
		// OK
	default:
		fmt.Fprintf(os.Stderr, "unknown output format %q, must be one of 'yaml' or 'kyaml'\n", *format)
		os.Exit(1)
	}
	if *diff && *write {
		fmt.Fprintln(os.Stderr, "cannot use -d and -w together")
	}

	files := fs.Args()

	if len(files) == 0 {
		if err := renderYAML(os.Stdin, *format, *diff, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	for i, path := range files {
		// use a func to catch defer'ed Close
		func() {
			// Read the YAML file
			sourceYaml, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
				return
			}
			in := bytes.NewReader(sourceYaml)

			out := os.Stdout
			finalize := func() {}
			if *write {
				// Write to a temp file and rename when done.
				tmp, err := os.CreateTemp("", "yamlfmt-")
				if err != nil {
					fmt.Fprintf(os.Stderr, "%v\n", err)
					os.Exit(1)
				}
				defer tmp.Close()
				finalize = func() {
					if err := os.Rename(tmp.Name(), path); err != nil {
						fmt.Fprintf(os.Stderr, "%v\n", err)
						os.Exit(1)
					}
				}
				out = tmp
			}
			if len(files) > 1 && !*write {
				if i > 0 {
					fmt.Fprintln(out, "")
				}
				fmt.Fprintln(out, "# "+path)
			}
			if err := renderYAML(in, *format, *diff, out); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			finalize()
		}()
	}
}

func renderYAML(in io.Reader, format string, printDiff bool, out io.Writer) error {
	if format == fmtKYAML {
		ky := &kyaml.Encoder{}

		if printDiff {
			ibuf, err := io.ReadAll(in)
			if err != nil {
				return err
			}
			obuf := bytes.Buffer{}
			if err := ky.FromYAML(bytes.NewReader(ibuf), &obuf); err != nil {
				return err
			}
			d := diff.Diff(string(ibuf), obuf.String())
			fmt.Fprint(out, d)
			return nil
		}

		return ky.FromYAML(in, out)
	}

	// else format == fmtYAML

	var decoder *yaml.Decoder
	var encoder *yaml.Encoder
	var finish func()

	if printDiff {
		ibuf, err := io.ReadAll(in)
		if err != nil {
			return err
		}
		obuf := bytes.Buffer{}
		decoder = yaml.NewDecoder(bytes.NewReader(ibuf))
		encoder = yaml.NewEncoder(&obuf)
		finish = func() {
			d := diff.Diff(string(ibuf), obuf.String())
			fmt.Fprint(out, d)
		}
	} else {
		decoder = yaml.NewDecoder(in)
		encoder = yaml.NewEncoder(out)
	}
	encoder.SetIndent(2)

	for {
		var node yaml.Node // to retain comments
		if err := decoder.Decode(&node); err != nil {
			if err == io.EOF {
				break // End of input
			}
			return fmt.Errorf("failed to decode input: %w", err)
		}
		setStyle(&node, 0) // there's not a const for "block style"
		if err := encoder.Encode(&node); err != nil {
			return fmt.Errorf("failed to encode node: %w", err)
		}
	}
	if finish != nil {
		finish()
	}
	return nil
}

func setStyle(node *yaml.Node, style yaml.Style) {
	node.Style = style
	for _, child := range node.Content {
		setStyle(child, style)
	}
}
