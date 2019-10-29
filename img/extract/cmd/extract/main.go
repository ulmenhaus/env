package main

import (
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

	ec := &EdgeCollector{}
	// HACK skip the first one because it's a package def
	err = ec.Collect(identifiers[1:])
	if err != nil {
		panic(err)
	}

	nc := &NodeCollector{}
	nc.Collect([]string{
		"github.com/ulmenhaus/env/img/jql/osm",
	})
}
