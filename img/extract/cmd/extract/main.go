package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ulmenhaus/env/img/extract/collector"
	"github.com/ulmenhaus/env/img/extract/format"
)

func main() {
	// TODO use CLI library
	args := append([]string{"list"}, os.Args[1:]...)
	buffer := bytes.NewBuffer([]byte{})
	lister := exec.Command("go", args...)
	lister.Stdout = buffer
	err := lister.Run()
	if err != nil {
		panic(err)
	}
	pkgs := strings.Split(buffer.String(), "\n")
	c, err := collector.NewCollector(pkgs)
	if err != nil {
		panic(err)
	}
	err = c.CollectAll()
	if err != nil {
		panic(err)
	}
	// TODO take mode and format from command-line
	graph := c.Graph(collector.ModePkg)
	serialized, err := json.MarshalIndent(format.FormatJQL(graph), "", "    ")
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s\n", string(serialized))
}
