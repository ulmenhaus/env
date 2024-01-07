package target

// this fixture defines symbols that have intra file dependencies only

const singleLinePrvConst = 0 // a private const defined on a single line
const SingleLinePubConst = 0 // a private const defined on a single line

const (
	multiLinePrvConst = 0 // a private const defined as a part of a multi-line definition
	MultiLinePubConst = 0 // a public const defined as a part of a multi-line definition
)

var singleLinePrvVar = 0 // a private var defined on a single line
var SingleLinePubVar = 0 // a private var defined on a single line

var (
	multiLinePrvVar = 0 // a private var defined as a part of a multi-line definition
	MultiLinePubVar = 0 // a public var defined as a part of a multi-line definition
)

var SlVar2var = []int{
	singleLinePrvConst,
	SingleLinePubVar,
} // a var that references other vars

// A SingleLineType is a type that is defined on a single line
type SingleLineType int

// A MultiLineType is a type that is defined on multiple lines
type MultiLineType struct {
	SimpleField     int            // a primitive field
	TypeToTypeField SingleLineType // a field that references another type
}

// A MultilineInterface is an interface that is defined on multiple lines
type MultiLineInterface interface {
	// PrimitiveMethod is defined only in terms of primitive types
	SimpleMethod(int) error

	// CompositeMethod has a user-defined input type
	CompositeMethod(SingleLineType)

	// CompositeReturn has a user-defined return type
	CompositeReturn() SingleLineType
}

// SingleLineFuncCompositeInput is a function defined on a single line
type SingleLineFuncCompositeInput func(SingleLineType)

// SingleLineFuncCompositeReturn is a function defined on a single line
type SingleLineFuncCompositeReturn func() SingleLineType

// MultiLineFunc is a multi-line function that references a const, var, type, and field
func MultiLineFunc() {
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

// MultiLineMethod is a multi-line method that references a const, var, type, and field
func (*MultiLineType) MultiLineMethod() {
	a := singleLinePrvConst
	b := SlVar2var
	c := MultiLineType{}
	d := c.SimpleField
	var e MultiLineInterface
	e.SimpleMethod(a + len(b) + d)
	var f SingleLineFuncCompositeInput
	f = func(SingleLineType){}
	f(SingleLineType(0))
}
