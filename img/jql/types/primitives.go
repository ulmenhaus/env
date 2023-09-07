package types

import (
	"fmt"
	"strconv"
	"strings"

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
	switch typed := i.(type) {
	case string:
		return String(string(s) + typed), nil
	case int:
		if string(s) == "" {
			return String("000"), nil
		}
		converted, err := strconv.Atoi(string(s))
		if err != nil {
			return nil, fmt.Errorf("Cannot add int to non-int string: %s", s)
		}
		sum := strconv.Itoa(converted + typed)
		if len(sum) >= len(s) || (converted + typed) < 0 {
			return String(sum), nil
		}
		return String(strings.Repeat("0", len(s)-len(sum)) + sum), nil
	}
	return nil, fmt.Errorf("Unsupported addition - string + %T", i)
}

// Encoded returns the String encoded as a string
func (s String) Encoded() storage.Primitive {
	return string(s)
}

// An Integer is a numerical value
type Integer int

// NewInteger returns a new string from the encoded data
func NewInteger(i interface{}, features map[string]interface{}) (Entry, error) {
	if i == nil {
		return Integer(0), nil
	}
	d, ok := i.(float64)
	if !ok {
		return nil, fmt.Errorf("failed to unpack int from: %#v", i)
	}
	return Integer(d), nil
}

// Format formats the integer
func (d Integer) Format(ft string) string {
	return strconv.Itoa(int(d))
}

// Reverse creates a new integer from the input
func (d Integer) Reverse(ft, input string) (Entry, error) {
	value, err := strconv.Atoi(input)
	if err != nil {
		return nil, err
	}
	return Integer(value), nil
}

// Compare returns true iff the given object is an integer and comes
// after the argument
func (d Integer) Compare(i interface{}) bool {
	entry, ok := i.(Integer)
	if !ok {
		return false
	}
	return entry > d
}

// Add adds to the string
func (d Integer) Add(i interface{}) (Entry, error) {
	addend, ok := i.(int)
	if !ok {
		return nil, fmt.Errorf("Integers can only be added with integers")
	}
	return Integer(int(d) + addend), nil
}

// Encoded returns the Integer encoded as a int
func (s Integer) Encoded() storage.Primitive {
	return int(s)
}
