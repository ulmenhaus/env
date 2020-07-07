package collector

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ulmenhaus/env/img/explore/models"
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
	pkgs []string

	finder   *DefinitionFinder
	graph    *models.EncodedGraph
	loc2node map[string]models.EncodedNode // maps node canonical location to copy of corresponding node
	structs  map[string]bool               // tracks UIDs for structs so they can be distinguished from other typs
	ifaces   map[string]bool               // tracks UIDs for interfaces so they can be distinguished from other typs
	mode     string
	logger   *log.Logger
}

func NewCollector(pkgs []string, logger *log.Logger) (*Collector, error) {
	finder, err := NewDefinitionFinder(pkgs)
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
			Subsystems: []models.EncodedSubsystem{},
		},
		loc2node: map[string]models.EncodedNode{},
		structs:  map[string]bool{},
		ifaces:   map[string]bool{},
		logger:   logger,
	}, nil
}

func (c *Collector) MapFiles(f func(pkg, short, path string) error) error {
	for _, pkg := range c.pkgs {
		glob := filepath.Join(GoPath, "src", pkg, "*.go")
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

func (c *Collector) CountFiles() (int, error) {
	count := 0
	err := c.MapFiles(func(_, _, _ string) error {
		count += 1
		return nil
	})
	return count, err
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
				nodes, structs, ifaces := NodesFromTypedef(pkg, short, path, typed)
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
	c.BuildSubsystems()
	return nil
}

func (c *Collector) CollectNodes() error {
	total, err := c.CountFiles()
	if err != nil {
		return err
	}
	current := 0
	fmt.Printf("%d files to process\n", total)
	return c.MapFiles(func(pkg, short, path string) error {
		fmt.Printf("Processing %d\n", current)
		current += 1
		return c.collectNodesInFile(pkg, short, path)
	})
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
				c.logger.Printf("Got error: %s\n", err)
				return true
			}

			dest, ok := c.loc2node[def.Canonical()]
			if !ok {
				c.logger.Printf("Got miss on %#v\n", n)
				return true
			}
			if dest.UID == source.UID {
				// every component will have a trival reference to itself which we ignore
				return true
			}
			edge := models.EncodedEdge{
				SourceUID: source.UID,
				DestUID:   dest.UID,
				Location:  ref,
			}
			c.graph.Relations[models.RelationReferences] = append(c.graph.Relations[models.RelationReferences], edge)
			return true
		})
		return nil
	})
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
