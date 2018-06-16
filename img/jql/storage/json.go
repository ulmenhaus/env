package storage

import (
	"encoding/json"
	"io"
	"io/ioutil"
)

// A JSONStore writes an encoded database as JSON
type JSONStore struct{}

// Write performs the database transformation to JSON
func (s *JSONStore) Write(dst io.Writer, db EncodedDatabase) error {
	b, err := json.MarshalIndent(db, "", "    ")
	if err != nil {
		return err
	}
	_, err = dst.Write(b)
	return err
}

// Read performs the database transformation from JSON
func (s *JSONStore) Read(src io.Reader) (EncodedDatabase, error) {
	// XXX could stream
	b, err := ioutil.ReadAll(src)
	if err != nil {
		return nil, err
	}
	d := EncodedDatabase{}
	return d, json.Unmarshal(b, &d)
}
