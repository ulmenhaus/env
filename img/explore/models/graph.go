package models

import "fmt"

const (
	RelationReferences string = "references"
	RootSystem         string = "root"
)

// An EncodedLocation represents the position within a file for a definition or reference
type EncodedLocation struct {
	Path   string `json:"path"`
	Offset uint   `json:"offset"` // offset for the identifier
	Start  uint   `json:"start"`  // the start of the correspnding node for the identified object
	End    uint   `json:"end"`    // the end of the corresponding node for the identified object
	Lines  uint   `json:"lines"`  // the total number of lines for the identified object
}

func (l EncodedLocation) Canonical() string {
	// NOTE assumes the Paqth is canonical (e.g. is an absolute path)
	return fmt.Sprintf("%s#%d", l.Path, l.Offset)
}

// Component is an abstraction over nodes and subsystems
type Component struct {
	UID         string          `json:"uid"`
	Kind        string          `json:"kind"`
	DisplayName string          `json:"display_name"`
	Description string          `json:"description"`
	Location    EncodedLocation `json:"location"`
}

type EncodedNode struct {
	Component
	Public bool `json:"public"`
}

type EncodedSubsystem struct {
	Component
	Parts []string `json:"parts"`
}

type EncodedGraph struct {
	Nodes      []EncodedNode                `json:"nodes"`
	Subsystems []EncodedSubsystem           `json:"subsystems"`
	Relations  map[string]([](EncodedEdge)) `json:"relations"`
}

type SystemGraph struct {
	encoded *EncodedGraph

	components map[string]Component         // Maps UID to associated Component struct
	contains   map[string](map[string]bool) // Maps each subsystem recursively to the UID of avery node and subsystem it contains
	inside     map[string](map[string]bool) // Reverse map for contains
	over       map[string](map[string]bool) // Reverse map of under
	under      map[string]string            // Maps node uid to the subsystem into which the node has been collapsed
}

type EncodedEdge struct {
	SourceUID string          `json:"source_uid"`
	DestUID   string          `json:"dest_uid"`
	Location  EncodedLocation `json:"location"`
}

func NewSystemGraph(encoded *EncodedGraph) *SystemGraph {
	components := map[string]Component{}
	over := map[string](map[string]bool){
		RootSystem: map[string]bool{},
	}
	under := map[string]string{}
	for _, node := range encoded.Nodes {
		components[node.UID] = node.Component
		over[node.UID] = map[string]bool{}
		over[RootSystem][node.UID] = true
		under[node.UID] = RootSystem
	}
	for _, ss := range encoded.Subsystems {
		components[ss.UID] = ss.Component
		over[ss.UID] = map[string]bool{}
		over[RootSystem][ss.UID] = true
		under[ss.UID] = RootSystem
	}
	contains := map[string](map[string]bool){}
	buildContainmaentGraph(encoded, contains)
	inside := reverse(contains)

	return &SystemGraph{
		encoded: encoded,

		components: components,
		contains:   contains,
		inside:     inside,

		under: under,
		over:  over,
	}
}

func buildContainmaentGraph(eg *EncodedGraph, c map[string](map[string]bool)) {
	contained := map[string]bool{}
	subsystems := map[string]EncodedSubsystem{}
	for _, ss := range eg.Subsystems {
		subsystems[ss.UID] = ss
	}

	var buildFrom func(start string) // to support recursive closure

	buildFrom = func(start string) {
		if _, ok := c[start]; ok {
			return
		}
		ss, ok := subsystems[start]
		if !ok {
			// it's a node
			contained[start] = true
			return
		}
		contains := map[string]bool{}
		for _, part := range ss.Parts {
			contained[part] = true
			buildFrom(part)
			contains[part] = true
			for target := range c[part] {
				contains[target] = true
			}
		}
		c[start] = contains
	}

	for _, ss := range eg.Subsystems {
		buildFrom(ss.UID)
	}
	// "root" will contain any dangling nodes and subsystems
	root := map[string]bool{}
	for _, node := range eg.Nodes {
		if _, ok := contained[node.UID]; !ok {
			root[node.UID] = true
		}
	}
	for _, ss := range eg.Subsystems {
		if _, ok := contained[ss.UID]; !ok {
			root[ss.UID] = true
		}
	}
	c[RootSystem] = root
}

func reverse(m map[string](map[string]bool)) map[string](map[string]bool) {
	r := map[string](map[string]bool){}

	for source, targets := range m {
		r[source] = map[string]bool{}
		for target := range targets {
			r[target] = map[string]bool{}
		}
	}

	for source, targets := range m {
		for target := range targets {
			r[target][source] = true
		}
	}
	return r
}

// Uncontained returns all descendants of a subsystem that are currently
// within another subsystem (usually root)
//
// NOTE this procedure will only look directly, not recursively, within the
// subsystem, but this is ok if containment is hierarchical
func (sg *SystemGraph) Uncontained(parentUID string, lookin string) []string {
	components := []string{}
	for containedUID := range sg.over[lookin] {
		if sg.contains[parentUID][containedUID] {
			components = append(components, containedUID)
		}
	}
	return components
}

func (sg *SystemGraph) MoveInto(uid string, target string) {
	current := sg.under[uid]
	sg.under[uid] = target
	delete(sg.over[current], uid)
	sg.over[target][uid] = true
}

func (sg *SystemGraph) Components(under string) []Component {
	components := []Component{}
	for uid := range sg.over[under] {
		components = append(components, sg.components[uid])
	}
	return components
}

func (sg *SystemGraph) Edges() []EncodedEdge {
	return nil
}

func (sg *SystemGraph) Parent(uid string) string {
	return sg.under[uid]
}
