package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"strings"
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
	c := NewCollector(pkgs)
	err = c.CollectNodes()
	if err != nil {
		panic(err)
	}
	serialized, err := json.MarshalIndent(c.Graph(), "", "    ")
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s\n", string(serialized))
}
