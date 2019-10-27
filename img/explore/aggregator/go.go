package aggregator

import (
	"fmt"
	"path"
)

// GoAggregator implements the Aggregator interface for golang projects
type GoAggregator struct {
}

func (ga *GoAggregator) Collect() (Aggregation, error) {
	return &GoAggregation{
		name: "root",
		children: map[string]*GoAggregation{
			"dir A": &GoAggregation{
				name: "root/dir A",
			},
			"dir B": &GoAggregation{
				name: "root/dir B",
			},
		},
	}, nil
}

type GoAggregation struct {
	children map[string]*GoAggregation
	parent   *GoAggregation
	name     string
}

func (ga *GoAggregation) Child(name string) (Aggregation, error) {
	child, ok := ga.children[name]
	if !ok {
		return nil, fmt.Errorf("no such child")
	}
	return child, nil
}

func (ga *GoAggregation) Children() []Aggregation {
	aggs := []Aggregation{}
	// TODO should sort by name and group dirs and files
	for _, agg := range ga.children {
		aggs = append(aggs, agg)
	}
	return aggs
}

func (ga *GoAggregation) Stats() map[string]string {
	return map[string]string{
		"lines": "42",
	}
}

func (ga *GoAggregation) DisplayName() string {
	return path.Base(ga.name)
}
