package collector

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
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

	finder   *DefinitionFinder
	graph    *models.EncodedGraph
	loc2node map[string]models.EncodedNode // maps node canonical location to copy of corresponding node
}

func NewCollector(pkgs []string) (*Collector, error) {
	finder, err := NewDefinitionFinder(&build.Default, pkgs)
	if err != nil {
		return nil, err
	}

	return &Collector{
		pkgs: pkgs,

		finder: finder,
		graph: &models.EncodedGraph{
			Nodes: []models.EncodedNode{},
			Relations: map[string]([]models.EncodedEdge){
				models.RelationReferences: []models.EncodedEdge{},
			},
		},
		loc2node: map[string]models.EncodedNode{},
	}, nil
}

func (c *Collector) MapFiles(f func(pkg, short, path string) error) error {
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
			err = f(pkg, short, path)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Collector) collectNodesInFile(pkg, short, path string) error {
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
		default:
			continue
		}
	}
	return nil
}

func (c *Collector) CollectAll() error {
	err := c.CollectNodes()
	if err != nil {
		return err
	}
	for _, node := range c.graph.Nodes {
		c.loc2node[node.Location.Canonical()] = node
	}
	err = c.CollectReferences()
	if err != nil {
		return err
	}
	return nil
}

func (c *Collector) CollectNodes() error {
	return c.MapFiles(c.collectNodesInFile)
}

func (c *Collector) CollectReferences() error {
	return c.MapFiles(func(pkg, short, path string) error {
		fset := token.NewFileSet()

		contents, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, "", string(contents), 0)
		if err != nil {
			return err
		}

		var source *models.EncodedNode

		ast.Inspect(f, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			loc := models.EncodedLocation{
				Path:   path,
				Offset: uint(n.Pos()) - 1, // HACK these appear to be one-indexed?
			}
			candidate, ok := c.loc2node[loc.Canonical()]
			if ok {
				source = &candidate
			}

			// if we've reached this point and still don't have a source then we are not yet
			// in a node so subsequent operations are meaningless
			if source == nil {
				return true
			}

			// double check that the current node is in the decl to rule out false positives
			// e.g. from types of decls that aren't accounted for
			if uint(id.Pos()-1) < source.Location.Start || uint(id.End()-1) > source.Location.End { // HACK these are one-indexed
				return true
			}
			ref := models.EncodedLocation{
				Path:   path,
				Offset: uint(id.NamePos),
			}
			def, err := c.finder.Find(&build.Default, ref)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Got error: %s\n", err)
				return true
			}

			dest, ok := c.loc2node[def.Canonical()]
			if !ok {
				fmt.Fprintf(os.Stderr, "Got miss on %#v\n", n)
				return true
			}
			if dest.UID == source.UID {
				// every component will have a trival reference to itself which we ignore
				return true
			}
			edge := models.EncodedEdge{
				SourceUID: source.UID,
				DestUID:   dest.UID,
				// TODO location
			}
			c.graph.Relations[models.RelationReferences] = append(c.graph.Relations[models.RelationReferences], edge)
			return true
		})
		return nil
	})
}

func (c *Collector) Graph() *models.EncodedGraph {
	return c.graph
}
