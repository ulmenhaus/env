package collector

import (
	"bytes"
	"fmt"
	"log"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ulmenhaus/env/img/explore/models"
)

func collectFromFixtures() (*models.EncodedGraph, error) {
	pkgs := []string{
		"github.com/ulmenhaus/env/img/extract/tests/fixtures/source",
		"github.com/ulmenhaus/env/img/extract/tests/fixtures/target",
	}
	errLog := bytes.NewBuffer([]byte{})
	logger := log.New(errLog, "prefix", log.LstdFlags)
	c, err := NewCollector(pkgs, logger, false)
	if err != nil {
		return nil, err
	}
	err = c.CollectAll()
	if err != nil {
		return nil, err
	}
	return c.Graph(ModePkg), nil
}

func formatEdge(e models.EncodedEdge) string {
	return fmt.Sprintf("%s -> %s", e.SourceUID, e.DestUID)
}

func TestCollectAll(t *testing.T) {
	graph, err := collectFromFixtures()
	require.NoError(t, err)

	t.Run("collect nodes", func(t *testing.T) {
		nodesByUID := map[string]models.EncodedNode{}
		for _, node := range graph.Nodes {
			nodesByUID[node.Component.UID] = node
		}

		for _, tc := range nodeTestCases {
			t.Run(fmt.Sprintf("case %s", tc.name), func(t *testing.T) {
				require.Contains(t, nodesByUID, tc.node.Component.UID)
				require.Equal(t, tc.node, nodesByUID[tc.node.Component.UID])
			})
		}
	})

	t.Run("collect edges", func(t *testing.T) {
		require.Contains(t, graph.Relations, models.RelationReferences)
		edges := graph.Relations[models.RelationReferences]
		edgesByUID := map[string]models.EncodedEdge{}
		for _, edge := range edges {
			edgesByUID[formatEdge(edge)] = edge
		}
		for _, tc := range edgeTestCases {
			t.Run(fmt.Sprintf("case %s", tc.name), func(t *testing.T) {
				uid := formatEdge(tc.edge)
				require.Contains(t, edgesByUID, uid)
				require.Equal(t, tc.edge, edgesByUID[uid])
			})
		}
	})

	t.Run("build subsystems", func(t *testing.T) {
		subsystemsByUID := map[string]models.EncodedSubsystem{}
		for _, ss := range graph.Subsystems {
			subsystemsByUID[ss.UID] = ss
		}
		for _, tc := range subsystemTestCases {
			t.Run(fmt.Sprintf("case %s", tc.name), func(t *testing.T) {
				require.Contains(t, subsystemsByUID, tc.subsystem.UID)
				require.Equal(t, tc.subsystem, subsystemsByUID[tc.subsystem.UID])
			})
		}
	})
}
