package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
)

type EdgeCollector struct {
}

func (ec *EdgeCollector) Collect(ids []*ast.Ident) error {
	return nil
	for _, id := range ids {
		fmt.Printf("Object '%s' at: %d\n", id.Name, id.NamePos)
		cmd := exec.Command("guru", "-json", "definition", fmt.Sprintf("jql/osm/mapper.go:#%d", id.NamePos))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			fmt.Printf("Got error: %s\n", err)
		}
	}
	return nil
}

type NodeCollector struct {
}

func (nc *NodeCollector) Collect(pkgs []string) error {
	gopath := os.Getenv("GOPATH")
	for _, pkg := range pkgs {
		glob := filepath.Join(gopath, "src", pkg, "*.go")
		// TODO would be good to be able to filter out test files
		paths, err := filepath.Glob(glob)
		if err != nil {
			return err
		}
		for _, path := range paths {
			fset := token.NewFileSet()
			contents, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}
			f, err := parser.ParseFile(fset, "", string(contents), 0)
			if err != nil {
				return err
			}
			fmt.Printf("%s has %d decls\n", path, len(f.Decls))
		}
	}
	return nil
}
