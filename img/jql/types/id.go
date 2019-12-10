package types

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/ulmenhaus/env/img/jql/storage"
)

// An ID is a random identifier for an entry
type ID string

// NewID returns a new ID from the encoded data
func NewID(i interface{}, features map[string]interface{}) (Entry, error) {
	strategy, ok := features["strategy"]
	if !ok {
		return nil, fmt.Errorf("ID schema must have a strategy")
	}
	len := 0
	switch strategy {
	case "hex":
		lenI, ok := features["length"]
		if !ok {
			return nil, fmt.Errorf("hex strategy must have a length")
		}
		lenF, ok := lenI.(float64)
		if !ok {
			return nil, fmt.Errorf ("length must be an int")
		}
		len = int(lenF)
	default:
		return nil, fmt.Errorf("unknown strategy: %s", strategy)
	}
	if i == nil {
		return ID(hexID(len)), nil
	}
	s, ok := i.(string)
	if !ok {
		return nil, fmt.Errorf("failed to unpack string from: %#v", i)
	}
	return ID(s), nil
}

func hexID(l int) string {
	rand.Seed(time.Now().UTC().UnixNano())
	s := ""
	for i := 0; i < l; i++ {
		// TODO(rabrams) inefficient -- could get 8 values out of this
		encoded := rand.Int() & 15
		if encoded >= 10 {
			s += string('a' + (encoded - 10))
		} else {
			s += string('0' + encoded)
		}
	}
	return s
}

// Format formats the ID
func (id ID) Format(ft string) string {
	return string(id)
}

// Reverse creates a new string from the input
func (id ID) Reverse(ft, input string) (Entry, error) {
	return ID(input), nil
}

// Compare returns true iff the given object is a string and comes
// lexicographically after this string
func (id ID) Compare(i interface{}) bool {
	entry, ok := i.(ID)
	if !ok {
		return false
	}
	return entry > id
}

// Add concatonates the provided string with the String
func (id ID) Add(i interface{}) (Entry, error) {
	return nil, fmt.Errorf("cannot increment an ID")
}

// Encoded returns the String encoded as a string
func (id ID) Encoded() storage.Primitive {
	return string(id)
}
