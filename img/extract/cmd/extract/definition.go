package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"

	"github.com/ulmenhaus/env/img/explore/models"
	"golang.org/x/tools/go/loader"
)

// DefinitionFinder essentially reimplements the go guru definition finder but
// with reuse of information across multiple jump checks
type DefinitionFinder struct {
	prog *loader.Program
}

// NewDefinitionFinder returns a DefinitionFinder with the provided packages imported
func NewDefinitionFinder(bc *build.Context, pkgs []string) (*DefinitionFinder, error) {
	conf := newLoaderConf(bc)
	for _, pkg := range pkgs {
		// TODO need to make test import configurable
		conf.ImportWithTests(pkg)
	}
	prog, err := conf.Load()
	if err != nil {
		return nil, err
	}
	return &DefinitionFinder{
		prog: prog,
	}, nil
}

// Find returns the location of the definition of the referenced identifier
func (df *DefinitionFinder) Find(bc *build.Context, ref models.EncodedLocation) (models.EncodedLocation, error) {
	obj, err := findReferencedObject(df.prog, ref)
	if err != nil {
		return models.EncodedLocation{}, err
	}

	if !obj.Pos().IsValid() {
		return models.EncodedLocation{}, fmt.Errorf("%s is built in", obj.Name())
	}

	pos := df.prog.Fset.Position(obj.Pos())
	return models.EncodedLocation{
		Path:   pos.Filename,
		Offset: uint(pos.Offset),
	}, nil
}

func findReferencedObject(lprog *loader.Program, ref models.EncodedLocation) (types.Object, error) {
	// Find the named file among those in the loaded program
	//
	// TODO can hash these for faster lookup and get rid of sameFile
	var file *token.File
	lprog.Fset.Iterate(func(f *token.File) bool {
		if sameFile(ref.Path, f.Name()) {
			file = f
			return false
		}
		return true
	})
	var obj types.Object
	if file == nil {
		return obj, fmt.Errorf("file %s not found in loaded program", ref.Path)
	}

	if ref.Offset >= uint(file.Size()) {
		return obj, fmt.Errorf("offset beyond file size")
	}

	pos := file.Pos(int(ref.Offset))
	info, path, _ := lprog.PathEnclosingInterval(pos, pos)
	if path == nil {
		return obj, fmt.Errorf("no syntax here")
	}
	id, _ := path[0].(*ast.Ident)
	if id == nil {
		return obj, fmt.Errorf("no identifier here")
	}
	// Look up the declaration of this identifier. If id is an anonymous field declaration,
	// it is both a use of a type and a def of a field; prefer the use in that case.
	obj = info.Uses[id]
	if obj == nil {
		obj = info.Defs[id]
		if obj == nil {
			// Happens for y in "switch y := x.(type)",
			// and the package declaration,
			return obj, fmt.Errorf("no object for identifier")
		}
	}
	return obj, nil
}

// newLoaderConf creates a loader conf with the build context and
// errors to be silently ignored.
// (Not suitable if SSA construction follows.)
func newLoaderConf(bc *build.Context) *loader.Config {
	ctxt := *bc // copy
	ctxt.CgoEnabled = false
	lconf := &loader.Config{Build: &ctxt}
	lconf.Build = &ctxt
	lconf.AllowErrors = true
	// AllErrors makes the parser always return an AST instead of
	// bailing out after 10 errors and returning an empty ast.File.
	lconf.ParserMode = parser.AllErrors
	lconf.TypeChecker.Error = func(err error) {}
	return lconf
}

// sameFile returns true if x and y have the same basename and denote
// the same file.
func sameFile(x, y string) bool {
	if filepath.Base(x) == filepath.Base(y) { // (optimisation)
		if xi, err := os.Stat(x); err == nil {
			if yi, err := os.Stat(y); err == nil {
				return os.SameFile(xi, yi)
			}
		}
	}
	return false
}
