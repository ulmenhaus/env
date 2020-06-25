package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/ulmenhaus/env/img/extract/collector"
	"github.com/ulmenhaus/env/img/extract/format"
)

var (
	stripPrefixes []string
)

var rootCmd = &cobra.Command{
	Use:   "extract [options] [pkg]...",
	Short: "Extract symbols and references from a code base",
	Long: `Extract will analyze a code base and find all unique objects
                  that exist within it as well as the symbolic references
                  that exist between them .`,
	Run: func(cmd *cobra.Command, pkgs []string) {
		if len(pkgs) == 0 {
			pkgs = []string{"./..."}
		}
		buffer := bytes.NewBuffer([]byte{})
		args := append([]string{"list"}, pkgs...)
		lister := exec.Command("go", args...)
		lister.Stdout = buffer
		err := lister.Run()
		if err != nil {
			panic(err)
		}
		allPkgs := strings.Split(buffer.String(), "\n")
		c, err := collector.NewCollector(allPkgs)
		if err != nil {
			panic(err)
		}
		err = c.CollectAll()
		if err != nil {
			panic(err)
		}
		// TODO take mode and format from command-line
		graph := c.Graph(collector.ModePkg)
		formatted := format.FormatJQL(graph, stripPrefixes, os.Getenv("TMUX_WINDOW_NAME"))
		serialized, err := json.MarshalIndent(formatted, "", "    ")
		if err != nil {
			panic(err)
		}
		err = ioutil.WriteFile(".project.json", serialized, os.ModePerm)
		if err != nil {
			panic(err)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringSliceVar(&stripPrefixes, "strip-prefix", []string{}, "a list of prefixes to strip from package names in the output")
}

func main() {
	rootCmd.Execute()
}
