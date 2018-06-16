package types

import "fmt"

// A String is an array of characters
type String string

// NewString returns a new string from the encoded data
func NewString(i interface{}) (Entry, error) {
	s, ok := i.(string)
	if !ok {
		return nil, fmt.Errorf("failed to unpack string from: %#v", i)
	}
	return String(s), nil
}

// Format formats the string
func (s String) Format(fmt string) string {
	return string(s)
}
