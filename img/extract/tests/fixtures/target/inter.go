package target

// this fixture defines symbols that have inter file dependencies only

// InterFileFunc is a multi-line function that references a const, var, type, and field
// in other files
func InterFileFunc() {
	a := singleLinePrvConst
	b := slVar2var
	c := MultiLineType{}
	d := c.SimpleField
	var e MultiLineInterface
	e.SimpleMethod(0)
	SingleLineFuncCompositeInput(SingleLineType(0))
	c.MultiLineMethod()
}
