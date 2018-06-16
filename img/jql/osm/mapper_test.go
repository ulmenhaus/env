package osm

import (
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ulmenhaus/env/img/jql/storage"
	"github.com/ulmenhaus/env/img/jql/types"
)

type mockStore struct {
	db storage.EncodedDatabase
}

func (ms *mockStore) Read(io.Reader) (storage.EncodedDatabase, error) {
	return ms.db, nil
}

func (ms *mockStore) Write(io.Writer, storage.EncodedDatabase) error {
	return nil
}

func TestLoad(t *testing.T) {
	cases := []struct {
		name     string
		db       storage.EncodedDatabase
		expected types.Database
	}{
		{
			name: "basic loading",
			db: storage.EncodedDatabase{
				"_schemata": storage.EncodedTable{
					"pages.url": storage.EncodedEntry{
						"primary": true,
						"type":    "string",
					},
					"tags.id": storage.EncodedEntry{
						"primary": true,
						// TODO should be "uuid"
						"type": "string",
					},
					"tags.url": storage.EncodedEntry{
						// TODO should be "foreign.pages.url"
						"type": "string",
					},
					"tags.desc": storage.EncodedEntry{
						"type": "string",
					},
				},
				"pages": storage.EncodedTable{
					"https://www.zoidberg.com": storage.EncodedEntry{},
				},
				"tags": storage.EncodedTable{
					"6149c1fe-e9ea-4afc-af7d-542e09af83e7": {
						"url":  "https://www.zoidberg.com",
						"desc": "#superlame",
					},
				},
			},
			expected: types.Database{
				"pages": types.Table{
					Columns: []string{"url"},
					Entries: map[string][]types.Entry{
						"https://www.zoidberg.com": []types.Entry{
							types.String("https://www.zoidberg.com"),
						},
					},
				},
				"tags": types.Table{
					Columns: []string{"desc", "id", "url"},
					Entries: map[string][]types.Entry{
						"6149c1fe-e9ea-4afc-af7d-542e09af83e7": []types.Entry{
							types.String("#superlame"),
							types.String("6149c1fe-e9ea-4afc-af7d-542e09af83e7"),
							types.String("https://www.zoidberg.com"),
						},
					},
				},
			},
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			ms := &mockStore{
				db: tc.db,
			}
			osm, err := NewObjectStoreMapper(ms)
			require.NoError(t, err)
			actual, err := osm.Load(nil)
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
		})
	}
}
