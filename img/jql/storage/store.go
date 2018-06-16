package storage

import "io"

// A Primitive must be an int, string, bool, or null
type Primitive interface{}

// An EncodedEntry represents a row when a database is being serialized for storage
type EncodedEntry map[string]Primitive

// An EncodedTable represents a table when a database is being serialized for storage
type EncodedTable map[string]EncodedEntry

// An EncodedDatabase represents a databse being serialized for storage
type EncodedDatabase map[string]EncodedTable

// A Store is an object that can serialize an encoded database to a specific format
type Store interface {
	// Write performs the database serialization
	// TODO pass by reference?
	Write(dst io.Writer, db EncodedDatabase) error
	// Write performs the database deserialization
	Read(src io.Reader) (EncodedDatabase, error)
}
