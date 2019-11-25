package collector

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/ulmenhaus/env/img/explore/models"
)

func pos2loc(path string, pos token.Pos, node ast.Node, lines uint) models.EncodedLocation {
	return models.EncodedLocation{
		Path:   path,
		Offset: uint(pos) - 1, // HACK these positions seem to be one-indexed?
		Start:  uint(node.Pos()) - 1,
		End:    uint(node.End()) - 1,
		Lines:  lines,
	}
}

func NodeFromFunc(pkg, short, path string, f *ast.FuncDecl) models.EncodedNode {
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
			panic(fmt.Errorf("Unknown type for typeX: %#v", typeX))
		}
		recv := id.Name
		kind = KindMethod
		name = fmt.Sprintf("%s.%s", recv, name)
	}
	if 'a' <= name[0] && name[0] <= 'z' {
		public = false
	}
	// HACK for funcs we do # statements + one line for signature and one line for end
	lines := uint(2)
	if f.Body != nil {
		lines += uint(len(f.Body.List))
	}

	return models.EncodedNode{
		Component: models.Component{
			UID:         fmt.Sprintf("%s.%s", pkg, name),
			DisplayName: fmt.Sprintf("%s.%s", short, name),
			Description: doc,
			Kind:        kind,
			Location:    pos2loc(path, f.Name.NamePos, f, lines),
		},
		Public: public,
	}
}

func NodesFromGlobal(pkg, short, path string, global *ast.GenDecl) []models.EncodedNode {
	var kind string

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
					UID:         fmt.Sprintf("%s.%s", pkg, name),
					DisplayName: fmt.Sprintf("%s.%s", short, name),
					Description: doc,
					Kind:        kind,
					// Using spec here instead of id in case the global references another
					// global. This can get ambiguous with multiple ids in the same spec.
					//
					// Globals can span multiple lines but we treat them as one statement so one line
					Location: pos2loc(path, id.NamePos, spec, 1),
				},
				Public: public,
			})
		}
	}

	return nodes
}

// NodesFromTypedef returns the nodes that belong to a GenDecl, a slice of UIDs for the
// structs in the decl, and a slice of UIDs for the interfaces in the decl
func NodesFromTypedef(pkg, short, path string, typed *ast.GenDecl) ([]models.EncodedNode, []string, []string) {
	kind := KindType
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

		uid := fmt.Sprintf("%s.%s", pkg, name)
		nodes = append(nodes, models.EncodedNode{
			Component: models.Component{
				UID:         uid,
				DisplayName: fmt.Sprintf("%s.%s", short, name),
				Description: doc,
				Kind:        kind,
				// HACK one line for definition and one for closing curly brace
				Location: pos2loc(path, tspec.Name.NamePos, spec, uint(2)),
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
							UID:         fmt.Sprintf("%s.%s.%s", pkg, name, fieldName.Name),
							DisplayName: fmt.Sprintf("%s.%s.%s", short, name, fieldName.Name),
							Description: fieldDoc,
							Kind:        KindField,
							// NOTE for multiple fields on the same line this is ambiguous
							Location: pos2loc(path, fieldName.NamePos, field, 1),
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
							UID:         fmt.Sprintf("%s.%s.%s", pkg, name, methodName.Name),
							DisplayName: fmt.Sprintf("%s.%s.%s", short, name, methodName.Name),
							Description: methodDoc,
							Kind:        KindMethod,
							Location:    pos2loc(path, methodName.NamePos, method, 1),
						},
						Public: public,
					})
				}
			}
		}
	}

	return nodes, structs, ifaces
}
