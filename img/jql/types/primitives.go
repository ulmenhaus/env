package types

import (
	"fmt"

	"github.com/ulmenhaus/env/img/jql/storage"
)

// A String is an array of characters
type String string

// NewString returns a new string from the encoded data
func NewString(i interface{}, features map[string]interface{}) (Entry, error) {
	if i == nil {
		return String(""), nil
	}
	s, ok := i.(string)
	if !ok {
		return nil, fmt.Errorf("failed to unpack string from: %#v", i)
	}
	return String(s), nil
}

// Format formats the string
func (s String) Format(ft string) string {
	return string(s)
}

// Reverse creates a new string from the input
func (s String) Reverse(ft, input string) (Entry, error) {
	return String(input), nil
}

// Compare returns true iff the given object is a string and comes
// lexicographically after this string
func (s String) Compare(i interface{}) bool {
	entry, ok := i.(String)
	if !ok {
		return false
	}
	return entry > s
}

// Add concatonates the provided string with the String
func (s String) Add(i interface{}) (Entry, error) {
	addend, ok := i.(string)
	if !ok {
		return nil, fmt.Errorf("Strings can only be concatonated with strings")
	}
	return String(string(s) + addend), nil
}

// Encoded returns the String encoded as a string
func (s String) Encoded() storage.Primitive {
	return string(s)
}
