package ui

import (
	"fmt"
	"sort"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/explore/models"
)

const (
	LanguageGo string = "go"
)

// MainViewMode is the current mode of the MainView.
// It determines which subviews are displayed
type MainViewMode int

const (
	MainViewModeListBar MainViewMode = iota
)

// A MainView is the overall view including a directory list
type MainView struct {
	graph *models.SystemGraph

	show      map[string]bool // determines which types of things will be shown
	subsystem *string         // the subsystem that's currently in view (nil means the whole system)
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(graph *models.SystemGraph, g *gocui.Gui) (*MainView, error) {
	mv := &MainView{
		graph: graph,
		show:  map[string]bool{},
	}
	return mv, nil
}

// Edit handles keyboard inputs while in table mode
func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	return
}

func (mv *MainView) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	header, err := g.SetView(HeaderView, 0, 0, maxX-1, 3)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	header.Clear()
	items, err := g.SetView(ItemsView, 0, 3, maxX-1, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	items.Clear()
	g.SetCurrentView(ItemsView)
	items.SelBgColor = gocui.ColorWhite
	items.SelFgColor = gocui.ColorBlack
	items.Highlight = true

	headerS, rows := mv.tabulatedItems()
	for _, row := range rows {
		fmt.Fprintf(items, "%s\n", row)
	}
	fmt.Fprintf(header, headerS)

	return nil
}

func map2slice(m map[string]bool) []string {
	s := []string{}
	for key := range m {
		s = append(s, key)
	}
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s
}

func (mv *MainView) tabulatedItems() (string, []string) {
	rows := []string{}
	for _, comp := range mv.graph.Components(mv.subsystem) {
		rows = append(rows, fmt.Sprintf("%s         %s", comp.Kind, comp.DisplayName))
	}
	return "Components", rows
}

func (mv *MainView) SetKeyBindings(g *gocui.Gui) error {
	err := g.SetKeybinding(ItemsView, 'k', gocui.ModNone, mv.cursorUp)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(ItemsView, 'j', gocui.ModNone, mv.cursorDown)
	if err != nil {
		return err
	}
	/*
		err = g.SetKeybinding(ItemsView, 's', gocui.ModNone, mv.saveContents)
			if err != nil {
				return err
			}
			if err := g.SetKeybinding(ItemsView, gocui.KeyEnter, gocui.ModNone, mv.logTime); err != nil {
				return err
			}
			err = g.SetKeybinding(ItemsView, 'w', gocui.ModNone, mv.openLink)
			if err != nil {
				return err
			}
	*/

	return nil
}

func (mv *MainView) cursorDown(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	cx, cy := v.Cursor()
	if err := v.SetCursor(cx, cy+1); err != nil {
		ox, oy := v.Origin()
		if err := v.SetOrigin(ox, oy+1); err != nil {
			return err
		}
	}
	return nil
}

func (mv *MainView) cursorUp(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	ox, oy := v.Origin()
	cx, cy := v.Cursor()
	if err := v.SetCursor(cx, cy-1); err != nil && oy > 0 {
		if err := v.SetOrigin(ox, oy-1); err != nil {
			return err
		}
	}
	return nil
}
