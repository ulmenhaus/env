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
	loc   *time.Location
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
	return t.In(loc).Format(ft)
}

// Reverse creates a new date from the input
func (d Date) Reverse(ft, input string) (Entry, error) {
	if ft == "" {
		ft = "02 Jan 2006"
	}
	var t time.Time
	if input == "" {
		t = time.Now()
	} else {
		noLoc, err := time.Parse(ft, input)
		if err != nil {
			return nil, err
		}
		t = time.Date(noLoc.Year(), noLoc.Month(), noLoc.Day(), noLoc.Hour(), noLoc.Minute(),
			noLoc.Second(), noLoc.Nanosecond(), loc)
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

// A Time denotes a specifc second in history by modeling as the
// number of seconds (positive or negative) since 1 January 1970 UTC
type Time int

// NewTime returns a new time from the encoded data
func NewTime(i interface{}, features map[string]interface{}) (Entry, error) {
	if i == nil {
		return Time(0), nil
	}
	n, ok := i.(float64)
	if !ok {
		return nil, fmt.Errorf("failed to unpack int from: %#v", i)
	}
	return Time(n), nil
}

// Format formats the time
func (t Time) Format(ft string) string {
	p := epoch.Add(time.Second * time.Duration(t))
	if ft == "" {
		ft = "02 Jan 2006 15:04:05"
	}
	return p.In(loc).Format(ft)
}

// Reverse creates a new time from the input
func (t Time) Reverse(ft, input string) (Entry, error) {
	if ft == "" {
		ft = "02 Jan 2006 15:04:05"
	}
	if input == "" {
		return Time(time.Now().Unix()), nil
	}
	noLoc, err := time.Parse(ft, input)
	if err != nil {
		return nil, err
	}
	p := time.Date(noLoc.Year(), noLoc.Month(), noLoc.Day(), noLoc.Hour(), noLoc.Minute(),
		noLoc.Second(), noLoc.Nanosecond(), loc)

	return Time(p.Sub(epoch) / time.Second), nil
}

// Compare returns true iff the given object is a Time and comes
// after this time
func (t Time) Compare(i interface{}) bool {
	entry, ok := i.(Time)
	if !ok {
		return false
	}
	return entry > t
}

// Add increments the Time by the provided number of days
func (t Time) Add(i interface{}) (Entry, error) {
	days, ok := i.(int)
	if !ok {
		return nil, fmt.Errorf("Times can only be incremented by integers")
	}
	return Time(int(t) + days*24*60*60), nil
}

// Encoded returns the Time encoded as a string
func (t Time) Encoded() storage.Primitive {
	return int(t)
}

// init as a HACK hard coding to PST for now
func init() {
	var err error
	loc, err = time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(fmt.Sprintf("failed to load location: %s", err))
	}
}
