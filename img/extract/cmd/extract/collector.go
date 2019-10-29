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

func (c *Collector) CollectNodes() error {
	gopath := os.Getenv("GOPATH")
	for _, pkg := range c.pkgs {
		glob := filepath.Join(gopath, "src", pkg, "*.go")
		// TODO would be good to be able to filter out test files
		paths, err := filepath.Glob(glob)
		if err != nil {
			return err
		}
		base := filepath.Base(pkg)
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
					doc := ""
					if typed.Doc != nil {
						doc = typed.Doc.Text()
					}
					public := true
					kind := KindFunction
					name := typed.Name.Name
					if typed.Recv != nil {
						typeX := typed.Recv.List[0].Type
						if star, ok := typeX.(*ast.StarExpr); ok {
							typeX = star.X
						}
						id, ok := typeX.(*ast.Ident)
						if !ok {
							return fmt.Errorf("Unknown type for typeX: %#v", typeX)
						}
						recv := id.Name
						kind = KindMethod
						name = fmt.Sprintf("%s.%s", recv, name)
					}
					if 'a' <= name[0] && name[0] <= 'z' {
						public = false
					}
					c.graph.Nodes = append(c.graph.Nodes, models.EncodedNode{
						Component: models.Component{
							UID:         fmt.Sprintf("%s.%s", pkg, name),
							DisplayName: fmt.Sprintf("%s.%s", base, name),
							Description: doc,
							Kind:        kind,
						},
						Public: public,
					})
				case *ast.GenDecl:
					kind := ""

					switch typed.Tok {
					case token.CONST, token.VAR:
						if typed.Tok == token.CONST {
							kind = KindConst
						} else {
							kind = KindVar
						}
						for _, spec := range typed.Specs {
							vspec, ok := spec.(*ast.ValueSpec)
							doc := ""
							if vspec.Comment != nil {
								doc = vspec.Comment.Text()
							}
							if !ok {
								return fmt.Errorf("Unknown value for const/var: %#v", spec)
							}
							for _, id := range vspec.Names {
								name := id.Name
								public := true
								if 'a' <= name[0] && name[0] <= 'z' {
									public = false
								}

								c.graph.Nodes = append(c.graph.Nodes, models.EncodedNode{
									Component: models.Component{
										UID:         fmt.Sprintf("%s.%s", pkg, name),
										DisplayName: fmt.Sprintf("%s.%s", base, name),
										Description: doc,
										Kind:        kind,
									},
									Public: public,
								})
							}
						}
					case token.TYPE:
						kind = KindType
					default:
						continue
					}
					for _, spec := range typed.Specs {
						fmt.Printf("%s -- %#v\n", kind, spec)
					}
				default:
					// fmt.Printf("decl: %#v\n", decl)
				}
			}
		}
	}
	return nil
}

func (c *Collector) Graph() *models.EncodedGraph {
	return c.graph
}
