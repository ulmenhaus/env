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

	"github.com/ulmenhaus/env/img/explore/models"
)

const (
	KindConst    string = "const"
	KindField    string = "field"
	KindFunction string = "function"
	KindMethod   string = "method"
	KindType     string = "type"
	KindVar      string = "var"
)

type Collector struct {
	pkgs []string

	graph *models.EncodedGraph
}

func NewCollector(pkgs []string) *Collector {
	return &Collector{
		pkgs: pkgs,

		graph: &models.EncodedGraph{
			Nodes: []models.EncodedNode{},
		},
	}
}

func (c *Collector) CollectEdges(ids []*ast.Ident) error {
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

func pos2loc(path string, pos token.Pos) string {
	return fmt.Sprintf("%s#%d", path, pos)
}

func (c *Collector) CollectNodes() error {
	gopath := os.Getenv("GOPATH")
	for _, pkg := range c.pkgs {
		glob := filepath.Join(gopath, "src", pkg, "*.go")
		// TODO would be good to be able to filter out test files
		paths, err := filepath.Glob(glob)
		if err != nil {
			return err
		}
		short := filepath.Base(pkg)
		for _, path := range paths {
			fset := token.NewFileSet()
			contents, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}
			f, err := parser.ParseFile(fset, "", string(contents), parser.ParseComments)
			if err != nil {
				return err
			}
			for _, decl := range f.Decls {
				switch typed := decl.(type) {
				case *ast.FuncDecl:
					c.graph.Nodes = append(c.graph.Nodes, NodeFromFunc(pkg, short, path, typed))
				case *ast.GenDecl:
					switch typed.Tok {
					case token.CONST, token.VAR:
						c.graph.Nodes = append(c.graph.Nodes, NodesFromGlobal(pkg, short, path, typed)...)
					case token.TYPE:
						c.graph.Nodes = append(c.graph.Nodes, NodesFromTypedef(pkg, short, path, typed)...)
					default:
						continue
					}
				}
			}
		}
	}
	return nil
}

func (c *Collector) Graph() *models.EncodedGraph {
	return c.graph
}
