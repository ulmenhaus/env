package ui

import (
	"fmt"
	"io"

	"github.com/jroimartin/gocui"
)

// A TableView is a gocui object for vizualizing tabular data
type TableView struct {
	Header []string
	Values [][]string
	Widths []int
	row    int
	column int
}

// Down moves the cursor down
func (tv *TableView) Down(g *gocui.Gui, v *gocui.View) error {
	if tv.row < len(tv.Values)-1 {
		tv.row++
	}
	return nil
}

// Up moves the cursor up
func (tv *TableView) Up(g *gocui.Gui, v *gocui.View) error {
	if tv.row > 0 {
		tv.row--
	}
	return nil
}

// Right moves the cursor right
func (tv *TableView) Right(g *gocui.Gui, v *gocui.View) error {
	if len(tv.Values) > 0 && tv.column < len(tv.Values[0])-1 {
		tv.column++
	}
	return nil
}

// Left moves the cursor left
func (tv *TableView) Left(g *gocui.Gui, v *gocui.View) error {
	if tv.column > 0 {
		tv.column--
	}
	return nil
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
