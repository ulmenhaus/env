package ui

import (
	"fmt"

	"github.com/jroimartin/gocui"
)

// A Table is a gocui object for vizualizing tabular data
type Table struct {
	Values [][]string
	Widths []int
	row    int
	column int
}

// Down moves the cursor down
func (t *Table) Down(g *gocui.Gui, v *gocui.View) error {
	if t.row < len(t.Values)-1 {
		t.row++
	}
	return nil
}

// Up moves the cursor up
func (t *Table) Up(g *gocui.Gui, v *gocui.View) error {
	if t.row > 0 {
		t.row--
	}
	return nil
}

// Right moves the cursor right
func (t *Table) Right(g *gocui.Gui, v *gocui.View) error {
	if len(t.Values) > 0 && t.column < len(t.Values[0])-1 {
		t.column++
	}
	return nil
}

// Left moves the cursor left
func (t *Table) Left(g *gocui.Gui, v *gocui.View) error {
	if t.column > 0 {
		t.column--
	}
	return nil
}

// Layout returns the gocui object
func (t *Table) Layout(g *gocui.Gui) error {
	// TODO paginate horizantally and vertically
	// Also figure out how to make the cursor disappear when inactive
	maxX, maxY := g.Size()
	v, err := g.SetView("table", 0, 0, maxX-1, maxY-1)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
	}

	content := ""
	for i, row := range t.Values {
		for j, val := range row {
			width := t.Widths[j]
			if len(val) >= width {
				val = val[:width]
			} else {
				diff := width - len(val)
				for k := 0; k < diff; k++ {
					val += " "
				}
			}

			if i == t.row && j == t.column {
				content += "> " + val + " "
			} else {
				content += "  " + val + " "
			}
		}
		content += "\n"
	}
	v.Clear()
	fmt.Fprintf(v, content)
	return nil
}

// GetSelected returns the selected object
func (t *Table) GetSelected() [][]string {
	return [][]string{
		{
			t.Values[t.row][t.column],
		},
	}
}
