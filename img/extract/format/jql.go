package format

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulmenhaus/env/img/explore/models"
)

type ComponentEntry struct {
	DisplayName string `json:"DisplayName"`
	Kind        string `json:"Kind"`
	LoC         int    `json:"LoC"`
	Parts       int    `json:"Parts"`
	RefsIn      int    `json:"RI"`
	RefsOut     int    `json:"RO"`
	RefsWithin  int    `json:"RW"`
	SrcLocation string `json:"SrcLocation"`
	SoParent    string `json:"SoParent"`
}

type ReferenceEntry struct {
	SDSource string `json:"SDSource"`
	SDDest   string `json:"SDDest"`
}

func schema() map[string]interface{} {
	return map[string]interface{}{
		// TODO for jql schema we use display name rather than UID --
		// in case there are two items with the same name and package name
		// this will be ambiguous so should distinguish
		"components.DisplayName": map[string]interface{}{
			"primary": true,
			"type":    "string",
		},
		"components.Kind": map[string]interface{}{
			"type": "string",
		},
		"components.LoC": map[string]interface{}{
			"type": "int",
		},
		"components.Parts": map[string]interface{}{
			"type": "int",
		},
		"components.RI": map[string]interface{}{
			"type": "int",
		},
		"components.RO": map[string]interface{}{
			"type": "int",
		},
		"components.RW": map[string]interface{}{
			"type": "int",
		},
		"components.SrcLocation": map[string]interface{}{
			"type": "string",
		},
		"components.SoParent": map[string]interface{}{
			"type": "foreign.components",
		},
		"references.Location": map[string]interface{}{
			"type":    "string",
			"primary": true,
		},
		"references.SDSource": map[string]interface{}{
			"type": "string",
		},
		"references.SDDest": map[string]interface{}{
			"type": "string",
		},
		"macros.Key": map[string]interface{}{
			"type":    "string",
			"primary": true,
		},
		"macros.Location": map[string]interface{}{
			"type": "string",
		},
		"macros.Params": map[string]interface{}{
			"type": "string",
		},
	}
}

func formatLocation(location models.EncodedLocation) string {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Sprintf("%s#%d", location.Path, location.Offset)
	}
	path, err := filepath.Rel(wd, location.Path)
	if err != nil {
		return fmt.Sprintf("%s#%d", location.Path, location.Offset)
	}
	return fmt.Sprintf("%s#%d", path, location.Offset)
}

func FormatJQL(g *models.EncodedGraph, stripPrefixes []string) map[string]interface{} {
	// NOTE jql model assumes subsystem relation is hierarchical
	// so no subsystem should be contained in more than one
	// parent system
	uid2ds := map[string]string{}
	stripPrefix := func(ds string) string {
		for _, prefix := range stripPrefixes {
			if strings.HasPrefix(ds, prefix) {
				return ds[len(prefix):]
			} else if fmt.Sprintf("%s/", ds) == prefix {
				return "."
			}
		}
		return ds
	}
	for _, en := range g.Nodes {
		uid2ds[en.UID] = stripPrefix(en.DisplayName)
	}
	for _, ss := range g.Subsystems {
		uid2ds[ss.UID] = stripPrefix(ss.DisplayName)
	}
	parents := map[string]string{}
	for _, parent := range g.Subsystems {
		for _, child := range parent.Parts {
			parents[child] = parent.UID
		}
	}
	m := map[string]interface{}{}
	m["_schemata"] = schema()
	components := map[string]ComponentEntry{}
	for _, en := range g.Nodes {
		components[uid2ds[en.UID]] = ComponentEntry{
			Kind:        en.Kind,
			LoC:         int(en.Location.Lines),
			SoParent:    uid2ds[parents[en.UID]],
			SrcLocation: formatLocation(en.Location),
		}
	}
	for _, ss := range g.Subsystems {
		components[uid2ds[ss.UID]] = ComponentEntry{
			Kind:        ss.Kind,
			SoParent:    uid2ds[parents[ss.UID]],
			SrcLocation: formatLocation(ss.Location),
		}
	}
	references := map[string]ReferenceEntry{}
	for _, ref := range g.Relations[models.RelationReferences] {
		references[formatLocation(ref.Location)] = ReferenceEntry{
			SDSource: uid2ds[ref.SourceUID],
			SDDest:   uid2ds[ref.DestUID],
		}
	}

	m["components"] = components
	m["references"] = references
	m["macros"] = map[string]interface{}{
		"E": map[string]interface{}{
			"Location": "jql-system-accumulator",
			"Params":   "",
		},
	}
	return m
}
