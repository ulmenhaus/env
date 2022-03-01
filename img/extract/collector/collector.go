package collector

import (
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ulmenhaus/env/img/explore/models"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

const (
	// node types
	KindConst    string = "const"
	KindField    string = "field"
	KindFunction string = "function"
	KindMethod   string = "method"
	KindTypename string = "typename"
	KindVar      string = "var"

	// subsystem types
	KindDir     string = "directory"
	KindFile    string = "file"
	KindGeneric string = "generic" // generic type (i.e. not a struct or an interface)
	KindIface   string = "interface"
	KindPkg     string = "package"
	KindStruct  string = "struct"

	ModePkg  string = "package"
	ModeFile string = "file"
)

var (
	GoPath = os.Getenv("GOPATH")
)

type Collector struct {
	nmes []string
	pkgs []*packages.Package

	graph       *models.EncodedGraph
	start2node  map[string]models.EncodedNode // maps node start location to copy of corresponding node
	offset2node map[string]models.EncodedNode // maps node offset to copy of corresponding node
	structs     map[string]bool               // tracks UIDs for structs so they can be distinguished from other typs
	ifaces      map[string]bool               // tracks UIDs for interfaces so they can be distinguished from other typs
	mode        string
	logger      *log.Logger
}

func NewCollector(names []string, logger *log.Logger, tests bool) (*Collector, error) {
	cfg := &packages.Config{
		// TODO(rabrams) Only a subset of these might be needed
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps | packages.NeedExportsFile | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedTypesSizes,
		// TODO(rabrams) Make this configurable since there is a noticeable perf cost
		Tests: tests,
	}
	pkgs, err := packages.Load(cfg, names...)
	if err != nil {
		return nil, err
	}
	return &Collector{
		pkgs: pkgs,

		graph: &models.EncodedGraph{
			Nodes: []models.EncodedNode{},
			Relations: map[string]([]models.EncodedEdge){
				models.RelationReferences: []models.EncodedEdge{},
			},
			Subsystems: []models.EncodedSubsystem{},
		},
		start2node:  map[string]models.EncodedNode{},
		offset2node: map[string]models.EncodedNode{},
		structs:     map[string]bool{},
		ifaces:      map[string]bool{},
		logger:      logger,
	}, nil
}

func (c *Collector) CollectAll() error {
	c.CollectNodes()
	for _, node := range c.graph.Nodes {
		c.start2node[node.Location.FullStart()] = node
		c.offset2node[node.Location.FullOffset()] = node
	}
	c.CollectReferences()
	c.BuildSubsystems()
	return nil
}

func (c *Collector) CollectNodes() {
	for _, pkg := range c.pkgs {
		for _, f := range pkg.Syntax {
			for _, decl := range f.Decls {
				switch typed := decl.(type) {
				case *ast.FuncDecl:
					c.graph.Nodes = append(c.graph.Nodes, NodeFromFunc(pkg, f, typed))
				case *ast.GenDecl:
					switch typed.Tok {
					case token.CONST, token.VAR:
						c.graph.Nodes = append(c.graph.Nodes, NodesFromGlobal(pkg, f, typed)...)
					case token.TYPE:
						nodes, structs, ifaces := NodesFromTypedef(pkg, f, typed)
						c.graph.Nodes = append(c.graph.Nodes, nodes...)
						for _, uid := range structs {
							c.structs[uid] = true
						}
						for _, uid := range ifaces {
							c.ifaces[uid] = true
						}
					default:
						continue
					}
				default:
					continue
				}
			}
		}
	}
}

func (c *Collector) CollectReferences() {
	path2pkg := map[string]*packages.Package{}
	for _, pkg := range c.pkgs {
		path2pkg[pkg.PkgPath] = pkg
	}
	for _, pkg := range c.pkgs {
		for _, f := range pkg.Syntax {
			pf := pkg.Fset.File(f.Pos())
			ast.Inspect(f, func(n ast.Node) bool {
				var source *models.EncodedNode
				if _, ok := n.(*ast.Ident); !ok {
					return true
				}
				path, _ := astutil.PathEnclosingInterval(f, n.Pos(), n.Pos())
				if path == nil {
					return true
				}
				// the source will be the last decl (e.g. ast.FuncDecl) just before the root ast.File
				if len(path) < 2 {
					return true
				}
				sourceToken := path[len(path)-2]

				// One-index as that's how an editor will reference it
				offset := (uint(n.Pos()) - uint(pf.Base())) + 1
				sourceOffset := (uint(sourceToken.Pos()) - uint(pf.Base())) + 1

				sourceLoc := models.EncodedLocation{
					Path:  pf.Name(),
					Start: sourceOffset,
				}
				loc := models.EncodedLocation{
					Path:  pf.Name(),
					Offset: offset,
				}
				candidate, ok := c.start2node[sourceLoc.FullStart()]
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
				if offset < source.Location.Start || offset > source.Location.End {
					return true
				}
				id, _ := path[0].(*ast.Ident)
				obj := pkg.TypesInfo.Uses[id]
				if obj == nil {
					obj = pkg.TypesInfo.Defs[id]
					if obj == nil {
						// Happens for y in "switch y := x.(type)",
						// and the package declaration,
						return true
					}
				}
				if obj.Pkg() != nil {
					tgtpkg := path2pkg[obj.Pkg().Path()]
					if tgtpkg != nil {
						tgtfile := tgtpkg.Fset.File(obj.Pos())
						tgtloc := models.EncodedLocation{
							Path: tgtfile.Name(),
							// One-index as that's how an editor will reference it
							// TODO this is a common calculation so factor it out
							Offset: (uint(obj.Pos()) - uint(tgtfile.Base())) + 1,
						}
						tgt, ok := c.offset2node[tgtloc.FullOffset()]
						if !ok {
							return true
						}
						if tgt.UID == source.UID {
							// every component will have a trival reference to itself which we ignore
							return true
						}
						edge := models.EncodedEdge{
							SourceUID: source.UID,
							DestUID:   tgt.UID,
							Location:  loc,
						}
						c.graph.Relations[models.RelationReferences] = append(c.graph.Relations[models.RelationReferences], edge)
						return true
					}
				}
				return true
			})
		}
	}

}

func (c *Collector) subsystemsByUID() map[string]*models.EncodedSubsystem {
	subsystems := map[string]*models.EncodedSubsystem{} // maps uid to subsystem

	for _, node := range c.graph.Nodes {
		// Technically will only work on Unix OSes but no intention of running on Windows soon
		pkg := filepath.Dir(node.UID) + "/" + strings.Split(filepath.Base(node.UID), ".")[0]
		file := pkg + "/" + filepath.Base(node.Location.Path)
		parts := strings.Split(pkg, "/")

		if node.Kind == KindTypename {
			if _, ok := c.structs[node.UID]; ok {
				uid := fmt.Sprintf("%s.type", node.UID)
				subsystems[uid] = &models.EncodedSubsystem{
					Component: models.Component{
						UID:         uid,
						Kind:        KindStruct,
						DisplayName: fmt.Sprintf("%s.type", node.DisplayName),
						Description: node.Description,
						Location:    node.Location,
					},
					Parts: []string{},
				}

			} else if _, ok := c.ifaces[node.UID]; ok {
				uid := fmt.Sprintf("%s.type", node.UID)
				subsystems[uid] = &models.EncodedSubsystem{
					Component: models.Component{
						UID:         uid,
						Kind:        KindIface,
						DisplayName: fmt.Sprintf("%s.type", node.DisplayName),
						Description: node.Description,
						Location:    node.Location,
					},
					Parts: []string{},
				}

			} else {
				uid := fmt.Sprintf("%s.type", node.UID)
				subsystems[uid] = &models.EncodedSubsystem{
					Component: models.Component{
						UID:         uid,
						Kind:        KindGeneric,
						DisplayName: fmt.Sprintf("%s.type", node.DisplayName),
						Description: node.Description,
						Location:    node.Location,
					},
					Parts: []string{},
				}
			}
		}

		if _, ok := subsystems[pkg]; !ok {
			subsystems[pkg] = &models.EncodedSubsystem{
				Component: models.Component{
					UID:         pkg,
					Kind:        KindPkg,
					DisplayName: pkg,
					Description: "", // TODO
					Location: models.EncodedLocation{
						Path: filepath.Dir(node.Location.Path),
					},
				},
				Parts: []string{},
			}
		}
		for i := range parts {
			dir := strings.Join(parts[:i+1], "/") + "/"
			if _, ok := subsystems[dir]; ok {
				continue
			}
			subsystems[dir] = &models.EncodedSubsystem{
				Component: models.Component{
					UID:         dir,
					Kind:        KindDir,
					DisplayName: dir,
					Description: "", // TODO
					Location: models.EncodedLocation{
						Path: filepath.Join(GoPath, "src", dir),
					},
				},
				Parts: []string{},
			}
		}
		if _, ok := subsystems[file]; !ok {
			subsystems[file] = &models.EncodedSubsystem{
				Component: models.Component{
					UID:         file,
					Kind:        KindFile,
					DisplayName: file,
					Description: "", // TODO
					Location: models.EncodedLocation{
						Path: node.Location.Path,
					},
				},
				Parts: []string{},
			}
		}
	}
	return subsystems
}

func (c *Collector) BuildSubsystems() {
	subsystems := c.subsystemsByUID()

	for _, ss := range subsystems {
		// Technically will only work on Unix OSes but no intention of running on Windows soon
		pkg := filepath.Dir(ss.Component.UID) + "/" + strings.Split(filepath.Base(ss.Component.UID), ".")[0]
		file := pkg + "/" + filepath.Base(ss.Component.Location.Path)

		switch ss.Component.Kind {
		case KindDir:
			container := filepath.Dir(filepath.Dir(ss.UID)) + "/"
			parent := subsystems[container]
			if parent != nil {
				parent.Parts = append(parent.Parts, ss.UID)
			}
		case KindFile:
			container := filepath.Dir(ss.UID) + "/"
			parent := subsystems[container]
			parent.Parts = append(parent.Parts, ss.UID)
		case KindIface, KindStruct, KindGeneric: // Technically a struct + methods can span multiple files so we could make it a part of the directory
			pkgParent := subsystems[pkg]
			pkgParent.Parts = append(pkgParent.Parts, ss.UID)
			fileParent := subsystems[file]
			fileParent.Parts = append(fileParent.Parts, ss.UID)
		case KindPkg:
			// ignore
		}
	}
	for _, node := range c.graph.Nodes {
		// Technically will only work on Unix OSes but no intention of running on Windows soon
		pkg := filepath.Dir(node.UID) + "/" + strings.Split(filepath.Base(node.UID), ".")[0]
		file := pkg + "/" + filepath.Base(node.Location.Path)

		switch node.Kind {
		case KindConst, KindFunction, KindVar:
			parentPkg := subsystems[pkg]
			parentPkg.Parts = append(parentPkg.Parts, node.UID)
			parentFile := subsystems[file]
			parentFile.Parts = append(parentFile.Parts, node.UID)
		case KindField, KindMethod:
			parentShort := strings.Join(strings.Split(filepath.Base(node.UID), ".")[:2], ".")
			parentType := fmt.Sprintf("%s/%s.type", filepath.Dir(node.UID), parentShort)
			parent := subsystems[parentType]
			parent.Parts = append(parent.Parts, node.UID)
			// Fields and methods will be subsystems of their parent types so will inherit their parent files and packages
			// parentFile := subsystems[file]
			// parentFile.Parts = append(parentFile.Parts, node.UID)
		case KindTypename:
			parent := subsystems[node.UID+".type"]
			parent.Parts = append(parent.Parts, node.UID)
		}
	}

	for _, subsystem := range subsystems {
		sort.Slice(subsystem.Parts, func(i, j int) bool { return subsystem.Parts[i] < subsystem.Parts[j] })
		c.graph.Subsystems = append(c.graph.Subsystems, *subsystem)
	}
}

func (c *Collector) Graph(mode string) *models.EncodedGraph {
	fitsMode := func(kind string) bool {
		if mode == ModeFile && kind == KindPkg {
			return false
		}
		if mode == ModePkg && (kind == KindFile || kind == KindDir) {
			return false
		}
		return true
	}

	eg := &models.EncodedGraph{
		Relations: map[string]([]models.EncodedEdge){},
	}
	included := map[string]bool{}
	for _, en := range c.graph.Nodes {
		if !fitsMode(en.Kind) {
			continue
		}
		eg.Nodes = append(eg.Nodes, en)
		included[en.UID] = true
	}

	for _, ss := range c.graph.Subsystems {
		if !fitsMode(ss.Kind) {
			continue
		}
		// NOTE assumes if an item fits the mode then
		// its parts do as well
		eg.Subsystems = append(eg.Subsystems, ss)
		included[ss.UID] = true
	}
	for name, relation := range c.graph.Relations {
		copy := []models.EncodedEdge{}
		for _, edge := range relation {
			if !included[edge.SourceUID] || !included[edge.DestUID] {
				continue
			}
			copy = append(copy, edge)
		}
		eg.Relations[name] = copy
	}
	return eg
}
