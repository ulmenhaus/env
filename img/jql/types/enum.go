package types

import (
	"fmt"
	"strings"

	"github.com/ulmenhaus/env/img/jql/storage"
)

// An Enum is a string that may have one of a fixed set of a values
// For convenience the value is held in memory as an int but for readability
// it is encoded as a string when stored
type Enum struct {
	values []string
	value  int
}

// NewEnum returns a new enum from the encoded data
func NewEnum(i interface{}, features map[string]interface{}) (Entry, error) {
	valuesI, ok := features["values"]
	if !ok {
		return Enum{}, fmt.Errorf("features for enum has no values")
	}
	valuesS, ok := valuesI.(string)
	if !ok {
		return Enum{}, fmt.Errorf("values must be a string")
	}
	values := strings.Split(valuesS, ", ")
	if i == nil {
		return Enum{
			values: values,
			value:  0,
		}, nil
	}
	iS, ok := i.(string)
	if !ok {
		return Enum{}, fmt.Errorf("failed to unpack Enum. Value is not a string")
	}
	for index, value := range values {
		if value == iS {
			return Enum{
				values: values,
				value:  index,
			}, nil
		}
	}
	return Enum{}, fmt.Errorf("failed to unpack Enum. Value is not present in values")
}

// Format formats the Enum
func (e Enum) Format(ft string) string {
	return e.values[e.value]
}

// Reverse creates a new Enum from the input
func (e Enum) Reverse(ft, input string) (Entry, error) {
	for index, value := range e.values {
		if input == value {
			return Enum{
				values: e.values,
				value:  index,
			}, nil
		}
	}
	return Enum{}, fmt.Errorf("invalid value for enum")
}

// Compare returns true iff the given object is a Enum and comes
// after this date
func (e Enum) Compare(i interface{}) bool {
	entry, ok := i.(Enum)
	if !ok {
		return false
	}
	return entry.value > e.value
}

// Add increments the Enum by the provided number of days
func (e Enum) Add(i interface{}) (Entry, error) {
	delta, ok := i.(int)
	if !ok {
		return nil, fmt.Errorf("Enums can only be incremented by integers")
	}
	posMod := func(i, modulus int) int {
		r := i % modulus
		if r < 0 {
			return r + modulus
		}
		return r
	}
	return Enum{
		values: e.values,
		value:  posMod(e.value+delta, len(e.values)),
	}, nil
}

// Encoded returns the Date encoded as a string
func (e Enum) Encoded() storage.Primitive {
	return e.values[e.value]
}
