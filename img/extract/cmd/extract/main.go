package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
)

func main() {
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

	c := NewCollector([]string{
		"github.com/ulmenhaus/env/img/jql/osm",
	})
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
