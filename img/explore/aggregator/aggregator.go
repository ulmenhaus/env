package aggregator

// An Aggregation is the result of an Aggregator doing its work
type Aggregation interface {
	Child(name string) (Aggregation, error)
	Children() []Aggregation
	Stats() map[string]string
	DisplayName() string
}

// An Aggregator gives an aggregate view of a project
type Aggregator interface {
	Collect() (Aggregation, error)
}
