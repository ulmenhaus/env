package ui

import (
	"fmt"
	"io"
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

// A TableView is a gocui object for vizualizing tabular data
type TableView struct {
	Header []string
	Values [][]string
	Widths []int
	row    int
	column int
}

// Move moves the table's cursor in the input direction
func (tv *TableView) Move(d Direction) {
	switch d {
	case DirectionRight:
		if len(tv.Values) > 0 && tv.column < len(tv.Values[0])-1 {
			tv.column++
		}
	case DirectionUp:
		if tv.row > 0 {
			tv.row--
		}
	case DirectionLeft:
		if tv.column > 0 {
			tv.column--
		}
	case DirectionDown:
		if tv.row < len(tv.Values)-1 {
			tv.row++
		}
	}
}

// WriteContents writes the contents of the table to a gocui view
func (tv *TableView) WriteContents(v io.Writer) error {
	// TODO paginate horizantally and vertically
	// Also figure out how to make the cursor disappear when inactive
	content := ""
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
		content += "  " + val + " "
	}
	content += "\n"
	for i, row := range tv.Values {
		for j, val := range row {
			width := tv.Widths[j]
			if len(val) >= width {
				val = val[:width]
			} else {
				diff := width - len(val)
				for k := 0; k < diff; k++ {
					val += " "
				}
			}

			if i == tv.row && j == tv.column {
				content += "> " + val + " "
			} else {
				content += "  " + val + " "
			}
		}
		content += "\n"
	}
	_, err := fmt.Fprintf(v, content)
	return err
}

// GetSelected returns the selected row and column
func (tv *TableView) GetSelected() (int, int) {
	return tv.row, tv.column
}
