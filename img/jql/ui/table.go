package ui

import (
	"fmt"
	"io"
	"strings"
)

// A Direction denotes which way to move a cursor when navigating
// a table view
type Direction int

const (
	// DirectionRight denotes moving the cursor right
	DirectionRight Direction = iota
	// DirectionUp denotes moving the cursor up
	DirectionUp
	// DirectionLeft denotes moving the cursor left
	DirectionLeft
	// DirectionDown denotes moving the cursor down
	DirectionDown
)

type Coordinate struct {
	Row    int
	Column int
}

type SelectionSet struct {
	Primary   Coordinate
	Secondary map[Coordinate]bool
	Tertiary  map[Coordinate]bool
}

// A TableView is a gocui object for vizualizing tabular data
type TableView struct {
	Header     []string
	Values     [][]string
	Widths     []int
	Selections SelectionSet
	XOffset    int
	TotalWidth int
}

// Move moves the table's cursor in the input direction
func (tv *TableView) Move(d Direction) {
	switch d {
	case DirectionRight:
		if len(tv.Values) > 0 && tv.Selections.Primary.Column < len(tv.Values[0])-1 {
			tv.Selections.Primary.Column++
		}
		tv.overflowRight()
	case DirectionUp:
		if tv.Selections.Primary.Row > 0 {
			tv.Selections.Primary.Row--
		}
	case DirectionLeft:
		if tv.Selections.Primary.Column > 0 {
			tv.Selections.Primary.Column--
		}
		tv.overflowLeft()
	case DirectionDown:
		if tv.Selections.Primary.Row < len(tv.Values)-1 {
			tv.Selections.Primary.Row++
		}
	}
}

// selectionLevel returns 0, 1, 2, or 3 if the given coordinate is
// unselected, primary selection, secondary, or tertiary respectively
func (tv *TableView) selectionLevel(c Coordinate) int {
	// A primary selection may also be a secondary and/or tertiary selection
	// but primary should take precedance over secondary &c
	if c == tv.Selections.Primary {
		return 1
	} else if tv.Selections.Secondary[c] {
		return 2
	} else if tv.Selections.Tertiary[c] {
		return 3
	} else {
		return 0
	}
}

// stringMult returns an input string repeated n times
func stringMult(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return s + stringMult(s, n-1)
}

// stringSlice hacks off the prefix of length n in a string while
// treating each unicode character as a single character
func stringSlice(s string, chars int) string {
	return string([]rune(s)[chars:])
}

// WriteContents writes the contents of the table to a gocui view
func (tv *TableView) WriteContents(v io.Writer) error {
	// TODO paginate horizantally and vertically
	// Also figure out how to make the cursor disappear when inactive
	header := ""
	for j, val := range tv.Header {
		width := tv.Widths[j]
		if len(val) >= width {
			val = val[:width]
		} else {
			diff := width - len(val)
			for k := 0; k < diff; k++ {
				val += " "
			}
		}
		header += "   " + val + stringMult(" ", 5)
	}
	header += "\n"
	content := stringSlice(header, tv.XOffset)
	for i, row := range tv.Values {
		rowString := ""
		for j, rawVal := range row {
			width := tv.Widths[j]
			rawVal = strings.Split(rawVal, "\n")[0]
			val := []rune(rawVal)
			if len(val) >= width {
				val = val[:width]
			} else {
				diff := width - len(val)
				for k := 0; k < diff; k++ {
					val = append(val, ' ')
				}
			}

			level := tv.selectionLevel(Coordinate{Row: i, Column: j})
			rowString += fmt.Sprintf("%s%s%s%s", stringMult(">", level), stringMult(" ", 3-level), string(val), stringMult(" ", 5))
		}
		content += stringSlice(rowString, tv.XOffset)
		content += "\n"
	}
	_, err := fmt.Fprintf(v, content)
	return err
}

// PrimarySelection returns the selected row and column
func (tv *TableView) PrimarySelection() (int, int) {
	return tv.Selections.Primary.Row, tv.Selections.Primary.Column
}

func (tv *TableView) SelectColumn() {
	for i := 0; i < len(tv.Values); i++ {
		// The primary selection is also added as a secondary
		tv.Selections.Secondary[Coordinate{Row: i, Column: tv.Selections.Primary.Column}] = true
	}
}

func (tv *TableView) SelectNone() {
	tv.Selections.Secondary = make(map[Coordinate]bool)
	tv.Selections.Tertiary = make(map[Coordinate]bool)
}

func (tv *TableView) overflowRight() {
	widthNeeded := 0
	for i:=0; i <= tv.Selections.Primary.Column; i ++ {
		widthNeeded += tv.Widths[i] + 8 // HACK 8 is our buffer size on each side
	}
	if widthNeeded > tv.TotalWidth {
		tv.XOffset = widthNeeded - tv.TotalWidth 
	}
}

func (tv *TableView) overflowLeft() {
	widthToCursor := 0
	for i:=0; i < tv.Selections.Primary.Column; i ++ {
		widthToCursor += tv.Widths[i] + 8 // HACK 8 is our buffer size on each side
	}
	if tv.XOffset > widthToCursor {
		tv.XOffset = widthToCursor
	}
}
