package target

// this fixture defines symbols that have inter file dependencies only

// InterFileFunc is a multi-line function that references a const, var, type, and field
// in other files
func InterFileFunc() {
	a := singleLinePrvConst
	b := SlVar2var
	c := MultiLineType{}
	d := c.SimpleField
	var e MultiLineInterface
	e.SimpleMethod(a + len(b) + d)
	var f SingleLineFuncCompositeInput
	f = func(SingleLineType){}
	f(SingleLineType(0))
	c.MultiLineMethod()
}
