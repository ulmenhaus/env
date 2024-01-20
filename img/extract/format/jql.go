package format

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ulmenhaus/env/img/extract/models"
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
	Test        string `json:"Test"`
	Top         string `json:"Top"`
}

type ReferenceEntry struct {
	SDSource string `json:"SDSource"`
	SDDest   string `json:"SDDest"`
}

func schema() map[string]interface{} {
	return map[string]interface{}{
		"bookmarks.Context": map[string]interface{}{
			"type": "string",
		},
		"bookmarks.Description": map[string]interface{}{
			"type": "string",
		},
		"bookmarks.Notes": map[string]interface{}{
			"type": "string",
		},
		"bookmarks.Location": map[string]interface{}{
			"type":    "string",
			"primary": true,
		},
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
		"base_references.Location": map[string]interface{}{
			"type":    "string",
			"primary": true,
		},
		"base_references.SDSource": map[string]interface{}{
			"type": "string",
		},
		"base_references.SDDest": map[string]interface{}{
			"type": "string",
		},
		"components.Test": map[string]interface{}{
			"type": "string",
		},
		"components.Top": map[string]interface{}{
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
		"macros.Reload": map[string]interface{}{
			"type": "enum",
			"features": map[string]interface{}{
				"values": "no, yes",
			},
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

func FormatJQL(g *models.EncodedGraph, stripPrefixes []string, projectName string) map[string]interface{} {
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
	// For jql schema we use display name rather than UID --
	// in case there are two items with the same name and package name
	// we disambiguate
	used := map[string]int{}
	for _, en := range g.Nodes {
		stripped := stripPrefix(en.DisplayName)
		used[stripped] += 1
	}
	for _, ss := range g.Subsystems {
		stripped := stripPrefix(ss.DisplayName)
		used[stripped] += 1
	}
	for _, en := range g.Nodes {
		stripped := stripPrefix(en.DisplayName)
		if used[stripped] > 1 {
			uid2ds[en.UID] = fmt.Sprintf("%s (%s)", stripPrefix(en.DisplayName), stripPrefix(en.UID))
		} else {
			uid2ds[en.UID] = stripPrefix(en.DisplayName)
		}
	}
	for _, ss := range g.Subsystems {
		stripped := stripPrefix(ss.DisplayName)
		if used[stripped] > 1 {
			uid2ds[ss.UID] = fmt.Sprintf("%s (%s)", stripPrefix(ss.DisplayName), stripPrefix(ss.UID))
		} else {
			uid2ds[ss.UID] = stripPrefix(ss.DisplayName)
		}
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
			Test:        bool2string(strings.HasSuffix(en.Location.Path, "_test.go")),
			Top:         bool2string(isPublic(en.UID)),
		}
	}
	for _, ss := range g.Subsystems {
		components[uid2ds[ss.UID]] = ComponentEntry{
			Kind:        ss.Kind,
			SoParent:    uid2ds[parents[ss.UID]],
			SrcLocation: formatLocation(ss.Location),
			Test:        bool2string(strings.HasSuffix(ss.Location.Path, "_test.go")),
			Top:         bool2string(ss.Kind != "struct" || isPublic(ss.UID)),
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
	m["base_references"] = references
	m["macros"] = map[string]interface{}{
		"E": map[string]interface{}{
			"Location": "jql-codedb-accumulate",
			"Params":   "",
		},
		"e": map[string]interface{}{
			"Location": "jql-codedb-edit",
			"Params":   "",
		},
		"R": map[string]interface{}{
			"Location": "jql-codedb-extract --strip-current-workdir",
			"Reload":   "yes",
		},
		"b": map[string]interface{}{
			"Location": "jql-codedb-extract --bookmarks-only",
			"Reload":   "yes",
		},
		"B": map[string]interface{}{
			"Location": "jql-codedb-filter",
			"Reload":   "no",
		},
	}
	bookmarks, err := GetProjectBookmarks(projectName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Got error collecting project bookmarks: %s\n", err)
	}
	m["bookmarks"] = bookmarks
	return m
}

func bool2string(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func isPublic(uid string) bool {
	parts := strings.Split(uid, ".")
	last := parts[len(parts)-1]
	if last == "type" {
		last = parts[len(parts) - 2]
	}
	return strings.ToUpper(last[:1]) == last[:1] && last[:1] != "_"
}
