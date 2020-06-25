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
	stripPrefixes       []string
	stripCurrentWorkdir bool
	bookmarksOnly       bool
)

var rootCmd = &cobra.Command{
	Use:   "extract [options] [pkg]...",
	Short: "Extract symbols and references from a code base",
	Long: `Extract will analyze a code base and find all unique objects
                  that exist within it as well as the symbolic references
                  that exist between them .`,
	Run: func(cmd *cobra.Command, pkgs []string) {
		if bookmarksOnly {
			contents, err := ioutil.ReadFile(".project.json")
			if err != nil {
				panic(err)
			}
			unmarshaled := map[string]interface{}{}
			err = json.Unmarshal(contents, &unmarshaled)
			if err != nil {
				panic(err)
			}
			bookmarks, err := format.GetProjectBookmarks(os.Getenv("TMUX_WINDOW_NAME"))
			if err != nil {
				panic(err)
			}
			unmarshaled["bookmarks"] = bookmarks
			marshaled, err := json.Marshal(unmarshaled)
			if err != nil {
				panic(err)
			}
			err = ioutil.WriteFile(".project.json", marshaled, os.ModePerm)
			if err != nil {
				panic(err)
			}
			return
		}
		if stripCurrentWorkdir {
			path, err := os.Getwd()
			if err == nil {
				stripPrefixes = append(stripPrefixes, path[len(os.Getenv("GOPATH")+"/src/"):]+"/")
			}
		}
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
	rootCmd.PersistentFlags().BoolVar(&stripCurrentWorkdir, "strip-current-workdir", false, "strip the current working directory from all package names")
	rootCmd.PersistentFlags().BoolVar(&bookmarksOnly, "bookmarks-only", false, "only update the bookmarks from the project file")
}

func main() {
	rootCmd.Execute()
}
