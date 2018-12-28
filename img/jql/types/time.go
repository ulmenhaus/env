package types

import (
	"fmt"
	"time"

	"github.com/ulmenhaus/env/img/jql/storage"
)

// A Date denotes a specifc day in history by modeling as the
// number of days (positive or negative) since 1 January 1970 UTC
type Date int

var (
	epoch = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
)

// NewDate returns a new date from the encoded data
func NewDate(i interface{}, features map[string]interface{}) (Entry, error) {
	if i == nil {
		return Date(0), nil
	}
	n, ok := i.(float64)
	if !ok {
		return nil, fmt.Errorf("failed to unpack int from: %#v", i)
	}
	return Date(n), nil
}

// Format formats the date
func (d Date) Format(ft string) string {
	t := epoch.Add(time.Hour * 24 * time.Duration(d))
	if ft == "" {
		ft = "02 Jan 2006"
	}
	return t.UTC().Format(ft)
}

// Reverse creates a new date from the input
func (d Date) Reverse(ft, input string) (Entry, error) {
	if ft == "" {
		ft = "02 Jan 2006"
	}
	t, err := time.Parse(ft, input)
	if err != nil {
		return nil, err
	}
	return Date(t.Sub(epoch) / (time.Hour * 24)), nil
}

// Compare returns true iff the given object is a Date and comes
// after this date
func (d Date) Compare(i interface{}) bool {
	entry, ok := i.(Date)
	if !ok {
		return false
	}
	return entry > d
}

// Add increments the Date by the provided number of days
func (d Date) Add(i interface{}) (Entry, error) {
	days, ok := i.(int)
	if !ok {
		return nil, fmt.Errorf("Dates can only be incremented by integers")
	}
	return Date(int(d) + days), nil
}

// Encoded returns the Date encoded as a string
func (d Date) Encoded() storage.Primitive {
	return int(d)
}
