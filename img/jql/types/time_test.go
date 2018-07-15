package types

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	cases := []struct {
		name     string
		date     Date
		format   string
		expected string
	}{
		{
			name:     "default format",
			date:     Date(0),
			format:   "",
			expected: "01 Jan 1970",
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			require.Equal(t, tc.expected, tc.date.Format(tc.format))
		})
	}
}
