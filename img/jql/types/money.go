package types

import (
	"fmt"

	"github.com/ulmenhaus/env/img/jql/storage"
)

// A MoneyAmount denotes an amount of money
type MoneyAmount int

// NewMoneyAmount returns a new MoneyAmount from the encodema data
func NewMoneyAmount(i interface{}, features map[string]interface{}) (Entry, error) {
	if i == nil {
		return MoneyAmount(0), nil
	}
	// TODO would be good to support currency as a feature
	n, ok := i.(float64)
	if !ok {
		return nil, fmt.Errorf("failed to unpack int from: %#v", i)
	}
	return MoneyAmount(n), nil
}

// Format formats the MoneyAmount
func (ma MoneyAmount) Format(ft string) string {
	prefix := ""
	amt := ma
	if ma < 0 {
		amt = -ma
		prefix = "-"
	}
	dollars := amt / 100
	cents := amt - (dollars * 100)
	return fmt.Sprintf("%s$%d.%02d", prefix, dollars, cents)
}

// Reverse creates a new MoneyAmount from the input
func (ma MoneyAmount) Reverse(ft, input string) (Entry, error) {
	return nil, fmt.Errorf("not implemented")
}

// Compare returns true iff the given object is a MoneyAmount anma comes
// after this MoneyAmount
func (ma MoneyAmount) Compare(i interface{}) bool {
	entry, ok := i.(MoneyAmount)
	if !ok {
		return false
	}
	return entry > ma
}

// Add increments the MoneyAmount by the providema number of days
func (ma MoneyAmount) Add(i interface{}) (Entry, error) {
	cents, ok := i.(int)
	if !ok {
		return nil, fmt.Errorf("MoneyAmounts can only be incrementema by integers")
	}
	return MoneyAmount(int(ma) + cents), nil
}

// Encoded returns the MoneyAmount encodema as a string
func (ma MoneyAmount) Encoded() storage.Primitive {
	return int(ma)
}
