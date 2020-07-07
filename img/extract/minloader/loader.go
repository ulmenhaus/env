// minloader is a minimal version of the loader library
// golang.org/x/tools/go/loader
//
// Original copyright notice:
// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package minloader

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/tools/go/ast/astutil"
)

var ignoreVendor build.ImportMode

const (
	trace = false
	// AllErrors makes the parser always return an AST instead of
	// bailing out after 10 errors and returning an empty ast.File.
	ParserMode = parser.AllErrors
)

var (
		BuildContext = &build.Default
)

type Config struct {
	Fset        *token.FileSet
	TypeChecker types.Config
	Cwd         string
	ImportPkgs  map[string]bool
	FindPackage func(ctxt *build.Context, importPath, fromDir string, mode build.ImportMode) (*build.Package, error)
}

type PkgSpec struct {
	Path      string
	Files     []*ast.File
	Filenames []string
}

type Program struct {
	Fset        *token.FileSet
	Created     []*PackageInfo
	Imported    map[string]*PackageInfo
	AllPackages map[*types.Package]*PackageInfo
	importMap   map[string]*types.Package
}

type PackageInfo struct {
	Pkg                   *types.Package
	Importable            bool
	TransitivelyErrorFree bool
	Files                 []*ast.File
	Errors                []error
	types.Info
	dir       string
	checker   *types.Checker
	errorFunc func(error)
}

func (info *PackageInfo) appendError(err error) {
	if info.errorFunc != nil {
		info.errorFunc(err)
	} else {
		fmt.Fprintln(os.Stderr, err)
	}
	info.Errors = append(info.Errors, err)
}

func (conf *Config) fset() *token.FileSet {
	if conf.Fset == nil {
		conf.Fset = token.NewFileSet()
	}
	return conf.Fset
}

func (conf *Config) ParseFile(filename string, src interface{}) (*ast.File, error) {
	return parser.ParseFile(conf.fset(), filename, src, ParserMode)
}

func (conf *Config) ImportWithTests(path string) { conf.addImport(path, true) }

func (conf *Config) Import(path string) { conf.addImport(path, false) }

func (conf *Config) addImport(path string, tests bool) {
	if path == "C" {
		return
	}
	if conf.ImportPkgs == nil {
		conf.ImportPkgs = make(map[string]bool)
	}
	conf.ImportPkgs[path] = conf.ImportPkgs[path] || tests
}

func (prog *Program) PathEnclosingInterval(start, end token.Pos) (pkg *PackageInfo, path []ast.Node, exact bool) {
	for _, info := range prog.AllPackages {
		for _, f := range info.Files {
			if f.Pos() == token.NoPos {
				continue
			}
			if !tokenFileContainsPos(prog.Fset.File(f.Pos()), start) {
				continue
			}
			if path, exact := astutil.PathEnclosingInterval(f, start, end); path != nil {
				return info, path, exact
			}
		}
	}
	return nil, nil, false
}

type importer struct {
	conf       *Config
	start      time.Time
	progMu     sync.Mutex
	prog       *Program
	findpkgMu  sync.Mutex
	findpkg    map[findpkgKey]*findpkgValue
	importedMu sync.Mutex
	imported   map[string]*importInfo
	graphMu    sync.Mutex
	graph      map[string]map[string]bool
}

type findpkgKey struct {
	importPath string
	fromDir    string
	mode       build.ImportMode
}

type findpkgValue struct {
	ready chan struct{}
	bp    *build.Package
	err   error
}

type importInfo struct {
	path     string
	info     *PackageInfo
	complete chan struct{}
}

func (ii *importInfo) awaitCompletion() {
	<-ii.complete
}

func (ii *importInfo) Complete(info *PackageInfo) {
	if info == nil {
		panic("info == nil")
	}
	ii.info = info
	close(ii.complete)
}

type importError struct {
	path string
	err  error
}

func (conf *Config) Load() (*Program, error) {
	if conf.TypeChecker.Error == nil {
		conf.TypeChecker.Error = func(e error) { fmt.Fprintln(os.Stderr, e) }
	}
	if conf.Cwd == "" {
		var err error
		conf.Cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	if conf.FindPackage == nil {
		conf.FindPackage = (*build.Context).Import
	}
	prog := &Program{
		Fset:        conf.fset(),
		Imported:    make(map[string]*PackageInfo),
		importMap:   make(map[string]*types.Package),
		AllPackages: make(map[*types.Package]*PackageInfo),
	}
	imp := importer{
		conf:     conf,
		prog:     prog,
		findpkg:  make(map[findpkgKey]*findpkgValue),
		imported: make(map[string]*importInfo),
		start:    time.Now(),
		graph:    make(map[string]map[string]bool),
	}
	var errpkgs []string
	infos, importErrors := imp.importAll("", conf.Cwd, conf.ImportPkgs, ignoreVendor)
	for _, ie := range importErrors {
		conf.TypeChecker.Error(ie.err)
		errpkgs = append(errpkgs, ie.path)
	}
	for _, info := range infos {
		prog.Imported[info.Pkg.Path()] = info
	}
	var xtestPkgs []*build.Package
	for importPath, augment := range conf.ImportPkgs {
		if !augment {
			continue
		}
		bp, err := imp.findPackage(importPath, conf.Cwd, ignoreVendor)
		if err != nil {
			continue
		}
		if len(bp.XTestGoFiles) > 0 {
			xtestPkgs = append(xtestPkgs, bp)
		}
		path := bp.ImportPath
		imp.importedMu.Lock()
		ii, ok := imp.imported[path]
		if !ok {
			panic(fmt.Sprintf("imported[%q] not found", path))
		}
		if ii == nil {
			panic(fmt.Sprintf("imported[%q] == nil", path))
		}
		if ii.info == nil {
			panic(fmt.Sprintf("imported[%q].info = nil", path))
		}
		info := ii.info
		imp.importedMu.Unlock()
		files, errs := imp.conf.parsePackageFiles(bp, 't')
		for _, err := range errs {
			info.appendError(err)
		}
		imp.addFiles(info, files, false)
	}
	createPkg := func(path, dir string, files []*ast.File, errs []error) {
		info := imp.newPackageInfo(path, dir)
		for _, err := range errs {
			info.appendError(err)
		}
		imp.addFiles(info, files, false)
		prog.Created = append(prog.Created, info)
	}
	for _, bp := range xtestPkgs {
		files, errs := imp.conf.parsePackageFiles(bp, 'x')
		createPkg(bp.ImportPath+"_test", bp.Dir, files, errs)
	}
	if len(prog.Imported)+len(prog.Created) == 0 {
		return nil, errors.New("no initial packages were loaded")
	}
	for _, obj := range prog.importMap {
		info := prog.AllPackages[obj]
		if info == nil {
			prog.AllPackages[obj] = &PackageInfo{Pkg: obj, Importable: true}
		} else {
			info.checker = nil
			info.errorFunc = nil
		}
	}
	markErrorFreePackages(prog.AllPackages)
	return prog, nil
}

func markErrorFreePackages(allPackages map[*types.Package]*PackageInfo) {
	importedBy := make(map[*types.Package]map[*types.Package]bool)
	for P := range allPackages {
		for _, Q := range P.Imports() {
			clients, ok := importedBy[Q]
			if !ok {
				clients = make(map[*types.Package]bool)
				importedBy[Q] = clients
			}
			clients[P] = true
		}
	}
	reachable := make(map[*types.Package]bool)
	var visit func(*types.Package)
	visit = func(p *types.Package) {
		if !reachable[p] {
			reachable[p] = true
			for q := range importedBy[p] {
				visit(q)
			}
		}
	}
	for _, info := range allPackages {
		if len(info.Errors) > 0 {
			visit(info.Pkg)
		}
	}
	for _, info := range allPackages {
		if !reachable[info.Pkg] {
			info.TransitivelyErrorFree = true
		}
	}
}

func (conf *Config) parsePackageFiles(bp *build.Package, which rune) ([]*ast.File, []error) {
	if bp.ImportPath == "unsafe" {
		return nil, nil
	}
	var filenames []string
	switch which {
	case 'g':
		filenames = bp.GoFiles
	case 't':
		filenames = bp.TestGoFiles
	case 'x':
		filenames = bp.XTestGoFiles
	default:
		panic(which)
	}
	return parseFiles(conf.fset(), BuildContext, bp.Dir, filenames, ParserMode)
}

func (imp *importer) doImport(from *PackageInfo, to string) (*types.Package, error) {
	if to == "C" {
		return nil, fmt.Errorf(`the loader doesn't cgo-process ad hoc packages like %q; see Go issue 11627`,
			from.Pkg.Path())
	}
	bp, err := imp.findPackage(to, from.dir, 0)
	if err != nil {
		return nil, err
	}
	if bp.ImportPath == "unsafe" {
		return types.Unsafe, nil
	}
	path := bp.ImportPath
	imp.importedMu.Lock()
	ii := imp.imported[path]
	imp.importedMu.Unlock()
	if ii == nil {
		panic("internal error: unexpected import: " + path)
	}
	if ii.info != nil {
		return ii.info.Pkg, nil
	}
	fromPath := from.Pkg.Path()
	if cycle := imp.findPath(path, fromPath); cycle != nil {
		pos, start := -1, ""
		for i, s := range cycle {
			if pos < 0 || s > start {
				pos, start = i, s
			}
		}
		cycle = append(cycle, cycle[:pos]...)[pos:]
		cycle = append(cycle, cycle[0])
		return nil, fmt.Errorf("import cycle: %s", strings.Join(cycle, " -> "))
	}
	panic("internal error: import of incomplete (yet acyclic) package: " + fromPath)
}

func (imp *importer) findPackage(importPath, fromDir string, mode build.ImportMode) (*build.Package, error) {
	key := findpkgKey{importPath, fromDir, mode}
	imp.findpkgMu.Lock()
	v, ok := imp.findpkg[key]
	if ok {
		imp.findpkgMu.Unlock()
		<-v.ready
	} else {
		v = &findpkgValue{ready: make(chan struct{})}
		imp.findpkg[key] = v
		imp.findpkgMu.Unlock()
		ioLimit <- true
		v.bp, v.err = imp.conf.FindPackage(BuildContext, importPath, fromDir, mode)
		<-ioLimit
		if _, ok := v.err.(*build.NoGoError); ok {
			v.err = nil
		}
		close(v.ready)
	}
	return v.bp, v.err
}

func (imp *importer) importAll(fromPath, fromDir string, imports map[string]bool, mode build.ImportMode) (infos []*PackageInfo, errors []importError) {
	var pending []*importInfo
	for importPath := range imports {
		bp, err := imp.findPackage(importPath, fromDir, mode)
		if err != nil {
			errors = append(errors, importError{
				path: importPath,
				err:  err,
			})
			continue
		}
		pending = append(pending, imp.startLoad(bp))
	}
	if fromPath != "" {
		imp.graphMu.Lock()
		deps, ok := imp.graph[fromPath]
		if !ok {
			deps = make(map[string]bool)
			imp.graph[fromPath] = deps
		}
		for _, ii := range pending {
			deps[ii.path] = true
		}
		imp.graphMu.Unlock()
	}
	for _, ii := range pending {
		if fromPath != "" {
			if cycle := imp.findPath(ii.path, fromPath); cycle != nil {
				if trace {
					fmt.Fprintf(os.Stderr, "import cycle: %q\n", cycle)
				}
				continue
			}
		}
		ii.awaitCompletion()
		infos = append(infos, ii.info)
	}
	return infos, errors
}

func (imp *importer) findPath(from, to string) []string {
	imp.graphMu.Lock()
	defer imp.graphMu.Unlock()
	seen := make(map[string]bool)
	var search func(stack []string, importPath string) []string
	search = func(stack []string, importPath string) []string {
		if !seen[importPath] {
			seen[importPath] = true
			stack = append(stack, importPath)
			if importPath == to {
				return stack
			}
			for x := range imp.graph[importPath] {
				if p := search(stack, x); p != nil {
					return p
				}
			}
		}
		return nil
	}
	return search(make([]string, 0, 20), from)
}

func (imp *importer) startLoad(bp *build.Package) *importInfo {
	path := bp.ImportPath
	imp.importedMu.Lock()
	ii, ok := imp.imported[path]
	if !ok {
		ii = &importInfo{path: path, complete: make(chan struct{})}
		imp.imported[path] = ii
		go func() {
			info := imp.load(bp)
			ii.Complete(info)
		}()
	}
	imp.importedMu.Unlock()
	return ii
}

func (imp *importer) load(bp *build.Package) *PackageInfo {
	fmt.Printf("Loading: %s/%s\n", bp.Dir, bp.Name)
	info := imp.newPackageInfo(bp.ImportPath, bp.Dir)
	info.Importable = true
	files, errs := imp.conf.parsePackageFiles(bp, 'g')
	for _, err := range errs {
		info.appendError(err)
	}
	imp.addFiles(info, files, true)
	imp.progMu.Lock()
	imp.prog.importMap[bp.ImportPath] = info.Pkg
	imp.progMu.Unlock()
	return info
}

func (imp *importer) addFiles(info *PackageInfo, files []*ast.File, cycleCheck bool) {
	var fromPath string
	if cycleCheck {
		fromPath = info.Pkg.Path()
	}
	imp.importAll(fromPath, info.dir, scanImports(files), 0)
	if trace {
		fmt.Fprintf(os.Stderr, "%s: start %q (%d)\n",
			time.Since(imp.start), info.Pkg.Path(), len(files))
	}
	if info.Pkg == types.Unsafe {
		if len(files) > 0 {
			panic(`"unsafe" package contains unexpected files`)
		}
	} else {
		info.checker.Files(files)
		info.Files = append(info.Files, files...)
	}
	if trace {
		fmt.Fprintf(os.Stderr, "%s: stop %q\n",
			time.Since(imp.start), info.Pkg.Path())
	}
}

func (imp *importer) newPackageInfo(path, dir string) *PackageInfo {
	var pkg *types.Package
	if path == "unsafe" {
		pkg = types.Unsafe
	} else {
		pkg = types.NewPackage(path, "")
	}
	info := &PackageInfo{
		Pkg: pkg,
		Info: types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Implicits:  make(map[ast.Node]types.Object),
			Scopes:     make(map[ast.Node]*types.Scope),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		},
		errorFunc: imp.conf.TypeChecker.Error,
		dir:       dir,
	}
	tc := imp.conf.TypeChecker
	tc.IgnoreFuncBodies = false
	tc.Importer = closure{imp, info}
	tc.Error = info.appendError
	info.checker = types.NewChecker(&tc, imp.conf.fset(), pkg, &info.Info)
	imp.progMu.Lock()
	imp.prog.AllPackages[pkg] = info
	imp.progMu.Unlock()
	return info
}

type closure struct {
	imp  *importer
	info *PackageInfo
}

func (c closure) Import(to string) (*types.Package, error) { return c.imp.doImport(c.info, to) }
