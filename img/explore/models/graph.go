package models

// Component is an abstraction over nodes and subsystems
type Component struct {
	UID         string `json:"uid"`
	Kind        string `json:"kind"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Location    string `json:"location"`
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
	Nodes      []EncodedNode             `json:"nodes"`
	Subsystems []EncodedSubsystem        `json:"subsystems"`
	Relations  map[string]([]([]string)) `json:"relations"`
}

type SystemGraph struct {
	encoded *EncodedGraph

	components map[string]Component // Maps UID to associated Component struct
	under      map[string]string    // Maps node uid to the subsystem the node is collapsed into
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

func (sg *SystemGraph) Edges() []Edge {
	return nil
}
