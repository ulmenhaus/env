package source

import "github.com/ulmenhaus/env/img/extract/tests/fixtures/target"

// this fixture defines symbols that have inter package dependencies only

// InterPackageFunc is a multi-line function that references a const, var, type, and field
// in other files
func InterPackageFunc() {
	a := target.SingleLinePubConst
	b := target.SlVar2var
	c := target.MultiLineType{}
	d := c.SimpleField
	var e target.MultiLineInterface
	e.SimpleMethod(a + len(b) + d)
	var f target.SingleLineFuncCompositeInput
	f = func(target.SingleLineType){}
	f(target.SingleLineType(0))
	c.MultiLineMethod()
}
