package models

import "fmt"

const (
	RelationReferences string = "references"
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

	components map[string]Component // Maps UID to associated Component struct
	under      map[string]string    // Maps node uid to the subsystem the node is collapsed into
}

type EncodedEdge struct {
	SourceUID string          `json:"source_uid"`
	DestUID   string          `json:"dest_uid"`
	Location  EncodedLocation `json:"location"`
}

func NewSystemGraph() *SystemGraph {
	return &SystemGraph{
		encoded: &EncodedGraph{
			Nodes:      []EncodedNode{},
			Subsystems: []EncodedSubsystem{},
			Relations:  map[string]([]EncodedEdge){},
		},
	}
}

func DecodeGraph(eg *EncodedGraph) *SystemGraph {
	graph := &SystemGraph{
		encoded: eg,

		components: map[string]Component{},
		under:      map[string]string{},
	}

	// At the start every node and subsystem will be shown under root aka ""
	for _, node := range eg.Nodes {
		graph.components[node.UID] = node.Component
		graph.under[node.UID] = ""
	}
	for _, subsystem := range eg.Subsystems {
		graph.components[subsystem.UID] = subsystem.Component
		graph.under[subsystem.UID] = ""
	}
	return graph
}

func (sg *SystemGraph) DeleteEntry(uid string) {
}

func (sg *SystemGraph) DeleteSubsystem(uid string) {
}

func (sg *SystemGraph) CollapseChildren(uid string) {
}

func (sg *SystemGraph) ExpandChildren(uid string) {
}

func (sg *SystemGraph) ExpandToLeaves(uid string) {
}

func (sg *SystemGraph) Components(under string) []Component {
	// Reversed map would be better here, but shmeh
	components := []Component{}
	for uid, parentUID := range sg.under {
		if parentUID != under {
			continue
		}
		components = append(components, sg.components[uid])
	}
	return components
}

func (sg *SystemGraph) Edges() []EncodedEdge {
	return nil
}
