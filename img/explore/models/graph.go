package models

// Component is an abstraction over nodes and subsystems
type Component struct {
	Kind        string `json:"kind"`
	DisplayName string `json:"display_name"`
}

type EncodedNode struct {
	Component
	UID         string `json:"uid"`
	Description string `json:"description"`
	Public      bool   `json:"bool"`
}

type EncodedSubsystem struct {
	Component
	UID         string   `json:"uid"`
	Description string   `json:"description"`
	Parts       []string `json:"parts"`
}

type EncodedGraph struct {
	Nodes      []EncodedNode             `json:"nodes"`
	Subsystems []EncodedSubsystem        `json:"subsystems"`
	Relations  map[string]([]([]string)) `json:"relations"`
}

type SystemGraph struct {
	encoded *EncodedGraph

	showas map[string]string // Maps node uid to the subsystem the node is shown as
}

type Edge struct {
	SourceDisplayName string
	DestDisplayName   string
}

func NewSystemGraph() *SystemGraph {
	return &SystemGraph{
		encoded: &EncodedGraph{
			Nodes:      []EncodedNode{},
			Subsystems: []EncodedSubsystem{},
			Relations:  map[string]([]([]string)){},
		},
	}
}

func DecodeGraph(eg *EncodedGraph) *SystemGraph {
	return &SystemGraph{
		encoded: eg,
	}
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

func (sg *SystemGraph) Components(root *string) []Component {
	components := []Component{}
	for _, node := range sg.encoded.Nodes {
		components = append(components, node.Component)
	}
	return components
}

func (sg *SystemGraph) Edges() []Edge {
	return nil
}
