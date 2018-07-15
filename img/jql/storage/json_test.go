package storage

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONWrite(t *testing.T) {
	cases := []struct {
		name     string
		db       EncodedDatabase
		expected string
	}{
		{
			name: "basic marshaling",
			db: EncodedDatabase{
				"_schemata": EncodedTable{
					"pages.url": EncodedEntry{
						"primary": true,
						"type":    "string",
					},
					"tags.id": EncodedEntry{
						"primary": true,
						"type":    "uuid",
					},
					"tags.url": EncodedEntry{
						"type": "foreign.web.key",
					},
					"tags.desc": EncodedEntry{
						"type": "string",
					},
					// TODO still needs testing for:
					// * formatters
					// * secondary indices
					// * non-string based types
					// * dynamic fields (.refs.count(), .refs.map())

					// NOTE should document that it's
					// inefficient to do a map for every
					// item, but makes for more readable files and
					// handles missing fields better
				},
				"tags": EncodedTable{
					"6149c1fe-e9ea-4afc-af7d-542e09af83e7": {
						"url":  "https://www.zoidberg.com",
						"desc": "#superlame",
					},
				},
				"pages": EncodedTable{
					"https://www.zoidberg.com": EncodedEntry{},
				},
			},
			expected: `{
    "_schemata": {
        "pages.url": {
            "primary": true,
            "type": "string"
        },
        "tags.desc": {
            "type": "string"
        },
        "tags.id": {
            "primary": true,
            "type": "uuid"
        },
        "tags.url": {
            "type": "foreign.web.key"
        }
    },
    "pages": {
        "https://www.zoidberg.com": {}
    },
    "tags": {
        "6149c1fe-e9ea-4afc-af7d-542e09af83e7": {
            "desc": "#superlame",
            "url": "https://www.zoidberg.com"
        }
    }
}`,
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			w := bytes.NewBuffer([]byte{})
			store := &JSONStore{}
			err := store.Write(w, tc.db)
			require.NoError(t, err)
			actual, err := ioutil.ReadAll(w)
			require.NoError(t, err)
			require.Equal(t, tc.expected, string(actual))
		})
	}
}

func TestJSONRead(t *testing.T) {
	cases := []struct {
		name     string
		contents string
		expected EncodedDatabase
	}{
		{
			name: "basic unmarshaling",
			contents: `{
    "_schemata": {
        "pages.url": {
            "primary": true,
            "type": "string"
        },
        "tags.desc": {
            "type": "string"
        },
        "tags.id": {
            "primary": true,
            "type": "uuid"
        },
        "tags.url": {
            "type": "foreign.web.key"
        }
    },
    "pages": {
        "https://www.zoidberg.com": {}
    },
    "tags": {
        "6149c1fe-e9ea-4afc-af7d-542e09af83e7": {
            "desc": "#superlame",
            "url": "https://www.zoidberg.com"
        }
    }
}`,
			expected: EncodedDatabase{
				"_schemata": EncodedTable{
					"pages.url": EncodedEntry{
						"primary": true,
						"type":    "string",
					},
					"tags.id": EncodedEntry{
						"primary": true,
						"type":    "uuid",
					},
					"tags.url": EncodedEntry{
						"type": "foreign.web.key",
					},
					"tags.desc": EncodedEntry{
						"type": "string",
					},
					// TODO still needs testing for:
					// * formatters
					// * secondary indices
					// * non-string based types
					// * dynamic fields (.refs.count(), .refs.map())

					// NOTE should document that it's
					// inefficient to do a map for every
					// item, but makes for more readable files and
					// handles missing fields better
				},
				"tags": EncodedTable{
					"6149c1fe-e9ea-4afc-af7d-542e09af83e7": {
						"url":  "https://www.zoidberg.com",
						"desc": "#superlame",
					},
				},
				"pages": EncodedTable{
					"https://www.zoidberg.com": EncodedEntry{},
				},
			},
		},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d-%s", i, tc.name), func(t *testing.T) {
			w := bytes.NewBuffer([]byte{})
			_, err := w.Write([]byte(tc.contents))
			require.NoError(t, err)
			store := &JSONStore{}
			db, err := store.Read(w)
			require.NoError(t, err)
			require.Equal(t, tc.expected, db)
		})
	}
}
