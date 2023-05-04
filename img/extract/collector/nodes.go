package collector

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/ulmenhaus/env/img/explore/models"
	"golang.org/x/tools/go/packages"
)

func pos2loc(path string, pos token.Pos, base uint, node ast.Node, lines uint) models.EncodedLocation {
	// one-index characters because that's how editors will reference them
	return models.EncodedLocation{
		Path:   path,
		Offset: uint(pos) + 1,
		Start:  (uint(node.Pos()) - base) + 1,
		End:    (uint(node.End()) - base) + 1,
		Lines:  lines,
	}
}

func NodeFromFunc(pkg *packages.Package, af *ast.File, f *ast.FuncDecl) *models.EncodedNode {
	pf := pkg.Fset.File(af.Pos())

	doc := ""
	if f.Doc != nil {
		doc = f.Doc.Text()
	}
	public := true
	kind := KindFunction
	name := f.Name.Name
	if f.Recv != nil {
		typeX := f.Recv.List[0].Type
		if star, ok := typeX.(*ast.StarExpr); ok {
			typeX = star.X
		}
		id, ok := typeX.(*ast.Ident)
		if !ok {
			fmt.Printf("Unknown type for typeX -- skipping: %#v\n", typeX)
			return nil
		}
		recv := id.Name
		kind = KindMethod
		name = fmt.Sprintf("%s.%s", recv, name)
	}
	if 'a' <= name[0] && name[0] <= 'z' {
		public = false
	}
	// HACK for funcs we do # chars / 27 + one line for signature and one line for end
	// so approximates LoC -- would be good to test margin of error on large code base
	lines := uint(2)
	if f.Body != nil {
		lines += uint((f.Body.Rbrace - f.Body.Lbrace) / 27)
	}

	return &models.EncodedNode{
		Component: models.Component{
			UID:         fmt.Sprintf("%s.%s", pkg.PkgPath, name),
			DisplayName: fmt.Sprintf("%s.%s", pkg.Name, name),
			Description: doc,
			Kind:        kind,
			Location:    pos2loc(pf.Name(), f.Name.NamePos - token.Pos(pf.Base()), uint(pf.Base()), f, lines),
		},
		Public: public,
	}
}

func NodesFromGlobal(pkg *packages.Package, f *ast.File, global *ast.GenDecl) []models.EncodedNode {
	var kind string

	pf := pkg.Fset.File(f.Pos())
	
	if global.Tok == token.CONST {
		kind = KindConst
	} else {
		kind = KindVar
	}

	nodes := []models.EncodedNode{}
	for _, spec := range global.Specs {
		vspec, ok := spec.(*ast.ValueSpec)
		doc := ""
		if vspec.Comment != nil {
			doc = vspec.Comment.Text()
		}
		if !ok {
			panic(fmt.Errorf("Unknown value for const/var: %#v", spec))
		}
		for _, id := range vspec.Names {
			name := id.Name
			public := true
			if 'a' <= name[0] && name[0] <= 'z' {
				public = false
			}

			nodes = append(nodes, models.EncodedNode{
				Component: models.Component{
					UID:         fmt.Sprintf("%s.%s", pkg.PkgPath, name),
					DisplayName: fmt.Sprintf("%s.%s", pkg.Name, name),
					Description: doc,
					Kind:        kind,
					// Using spec here instead of id in case the global references another
					// global. This can get ambiguous with multiple ids in the same spec.
					//
					// Globals can span multiple lines but we treat them as one statement so one line
					Location: pos2loc(pf.Name(), id.NamePos - token.Pos(pf.Base()), uint(pf.Base()), spec, 1),
				},
				Public: public,
			})
		}
	}

	return nodes
}

// NodesFromTypedef returns the nodes that belong to a GenDecl, a slice of UIDs for the
// structs in the decl, and a slice of UIDs for the interfaces in the decl
func NodesFromTypedef(pkg *packages.Package, f *ast.File, typed *ast.GenDecl) ([]models.EncodedNode, []string, []string) {
	pf := pkg.Fset.File(f.Pos())

	kind := KindTypename
	nodes := []models.EncodedNode{}
	structs := []string{}
	ifaces := []string{}

	for _, spec := range typed.Specs {
		tspec, ok := spec.(*ast.TypeSpec)
		if !ok {
			panic(fmt.Errorf("Unknown type for processing types: %#v", spec))
		}
		doc := ""
		if tspec.Comment != nil {
			doc = tspec.Comment.Text()
		}
		public := true
		name := tspec.Name.Name
		if 'a' <= name[0] && name[0] <= 'z' {
			public = false
		}

		uid := fmt.Sprintf("%s.%s", pkg.PkgPath, name)
		nodes = append(nodes, models.EncodedNode{
			Component: models.Component{
				UID:         uid,
				DisplayName: fmt.Sprintf("%s.%s", pkg.Name, name),
				Description: doc,
				Kind:        kind,
				// HACK one line for definition and one for closing curly brace
				Location: pos2loc(pf.Name(), tspec.Name.NamePos - token.Pos(pf.Base()), uint(pf.Base()), spec, uint(2)),
			},
			Public: public,
		})
		switch typeTyped := tspec.Type.(type) {
		case *ast.StructType:
			structs = append(structs, uid)
			for _, field := range typeTyped.Fields.List {
				fieldDoc := ""
				if field.Comment != nil {
					fieldDoc = field.Comment.Text()
				}
				for _, fieldName := range field.Names {
					nodes = append(nodes, models.EncodedNode{
						Component: models.Component{
							UID:         fmt.Sprintf("%s.%s.%s", pkg.PkgPath, name, fieldName.Name),
							DisplayName: fmt.Sprintf("%s.%s.%s", pkg.Name, name, fieldName.Name),
							Description: fieldDoc,
							Kind:        KindField,
							// NOTE for multiple fields on the same line this is ambiguous
							Location: pos2loc(pf.Name(), fieldName.NamePos - token.Pos(pf.Base()), uint(pf.Base()), field, 1),
						},
						Public: public,
					})
				}
			}
		case *ast.InterfaceType:
			ifaces = append(ifaces, uid)
			for _, method := range typeTyped.Methods.List {
				methodDoc := ""
				if method.Comment != nil {
					methodDoc = method.Comment.Text()
				}
				for _, methodName := range method.Names {
					nodes = append(nodes, models.EncodedNode{
						Component: models.Component{
							UID:         fmt.Sprintf("%s.%s.%s", pkg.PkgPath, name, methodName.Name),
							DisplayName: fmt.Sprintf("%s.%s.%s", pkg.Name, name, methodName.Name),
							Description: methodDoc,
							Kind:        KindMethod,
							Location:    pos2loc(pf.Name(), methodName.NamePos - token.Pos(pf.Base()), uint(pf.Base()), method, 1),
						},
						Public: public,
					})
				}
			}
		}
	}

	return nodes, structs, ifaces
}
