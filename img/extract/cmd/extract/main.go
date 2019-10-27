package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

func main() {
	// TODO use CLI library
	fset := token.NewFileSet()

	contents, err := ioutil.ReadFile("/Users/rabrams/src/github.com/ulmenhaus/env/img/jql/osm/mapper.go")
	if err != nil {
		panic(err)
	}
	f, err := parser.ParseFile(fset, "", string(contents), 0)
	if err != nil {
		fmt.Println(err)
		return
	}

	/*for _, s := range f.Decls {
		fmt.Printf("%#v\n\n", s)
	}*/

	identifiers := []*ast.Ident{}

	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		identifiers = append(identifiers, id)
		return true
	})

	args := append([]string{"list"}, os.Args[1:]...)
	buffer := bytes.NewBuffer([]byte{})
	lister := exec.Command("go", args...)
	lister.Stdout = buffer
	err = lister.Run()
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
