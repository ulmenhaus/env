package main

import (
	"fmt"
	"go/ast"
	"go/token"

	"github.com/ulmenhaus/env/img/explore/models"
)

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
	return models.EncodedNode{
		Component: models.Component{
			UID:         fmt.Sprintf("%s.%s", pkg, name),
			DisplayName: fmt.Sprintf("%s.%s", short, name),
			Description: doc,
			Kind:        kind,
			Location:    pos2loc(path, f.Name.NamePos),
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
					Location:    pos2loc(path, id.NamePos),
				},
				Public: public,
			})
		}
	}

	return nodes
}

func NodesFromTypedef(pkg, short, path string, typed *ast.GenDecl) []models.EncodedNode {
	kind := KindType
	nodes := []models.EncodedNode{}

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

		nodes = append(nodes, models.EncodedNode{
			Component: models.Component{
				UID:         fmt.Sprintf("%s.%s", pkg, name),
				DisplayName: fmt.Sprintf("%s.%s", short, name),
				Description: doc,
				Kind:        kind,
				Location:    pos2loc(path, tspec.Name.NamePos),
			},
			Public: public,
		})
		switch typeTyped := tspec.Type.(type) {
		case *ast.StructType:
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
							Location:    pos2loc(path, fieldName.NamePos),
						},
						Public: public,
					})
				}
			}
		case *ast.InterfaceType:
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
							Location:    pos2loc(path, methodName.NamePos),
						},
						Public: public,
					})
				}
			}
		}
	}

	return nodes
}
