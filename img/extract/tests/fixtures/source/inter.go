package source

import "github.com/ulmenhaus/env/img/extract/tests/fixtures/target"

// this fixture defines symbols that have inter package dependencies only

// InterPackageFunc is a multi-line function that references a const, var, type, and field
// in other files
func InterPackageFunc() {
	a := target.singleLinePrvConst
	b := target.slVar2var
	c := target.MultiLineType{}
	d := c.SimpleField
	var e target.MultiLineInterface
	e.SimpleMethod(0)
	target.SingleLineFuncCompositeInput(target.SingleLineType(0))
	c.MultiLineMethod()
}
