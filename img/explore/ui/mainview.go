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
	MainViewModeToggleShow
)

// A MainView is the overall view including a directory list
type MainView struct {
	graph *models.SystemGraph

	show      map[string]bool // determines which types of things will be shown
	subsystem string          // the subsystem that's currently in view (nil means the whole system)

	mode MainViewMode
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(graph *models.SystemGraph, g *gocui.Gui) (*MainView, error) {
	mv := &MainView{
		graph: graph,
		show:  map[string]bool{},
	}
	for _, comp := range graph.Components("") {
		mv.show[comp.Kind] = true
	}
	return mv, nil
}

// Edit handles keyboard inputs while in table mode
func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	return
}

func (mv *MainView) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	header, err := g.SetView(HeaderView, 0, 0, maxX/2, 3)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	header.Clear()
	items, err := g.SetView(ItemsView, 0, 3, maxX/2, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	items.Clear()
	detail, err := g.SetView(DetailView, maxX/2+1, 0, maxX-1, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	detail.Clear()

	if mv.mode == MainViewModeToggleShow {
		showToggle, err := g.SetView(ShowView, maxX/2-30, maxY/2-10, maxX/2+30, maxY/2+10)
		if err != nil && err != gocui.ErrUnknownView {
			return err
		}
		showToggle.Clear()
		showToggleContents := mv.showToggleContents()
		fmt.Fprintf(showToggle, showToggleContents)
		showToggle.SelBgColor = gocui.ColorWhite
		showToggle.SelFgColor = gocui.ColorBlack
		showToggle.Highlight = true
		err = mv.setKeyBindings(ShowView, g)
		if err != nil {
			return err
		}
		g.SetCurrentView(ShowView)
	} else {
		err := g.DeleteView(ShowView)
		if err != nil && err != gocui.ErrUnknownView {
			return err
		}
		g.SetCurrentView(ItemsView)
	}

	items.SelBgColor = gocui.ColorWhite
	items.SelFgColor = gocui.ColorBlack
	items.Highlight = true

	components := mv.graph.Components(mv.subsystem)
	headerS, rows := mv.tabulatedItems(components)
	for _, row := range rows {
		fmt.Fprintf(items, "%s\n", row)
	}
	fmt.Fprintf(header, headerS)
	_, oy := items.Origin()
	_, cy := items.Cursor()
	detailContents := mv.detailContents(components, oy+cy)
	fmt.Fprintf(detail, detailContents)

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

func multiplyS(s string, n int) string {
	toret := ""
	for i := 0; i < n; i++ {
		toret += s
	}
	return toret
}

func (mv *MainView) tabulatedItems(components []models.Component) (string, []string) {
	// TODO would be good to make a general purpose tabulator or use a standard one
	// XXX sorting an input slice
	sort.Slice(components, func(i, j int) bool { return components[i].DisplayName < components[j].DisplayName })
	rows := []string{}
	maxKind := 0
	loc := uint(0)
	for _, comp := range components {
		if len(comp.Kind) > maxKind {
			maxKind = len(comp.Kind)
		}
		loc += comp.Location.Lines
	}
	maxKind += 10 // padding
	for _, comp := range components {
		rows = append(rows, fmt.Sprintf("%s%s%s", comp.Kind, multiplyS(" ", maxKind-len(comp.Kind)), comp.DisplayName))
	}
	system := mv.subsystem
	if system == "" {
		system = "root system"
	}
	header := fmt.Sprintf(
		"%s\n %d of %d Nodes     %d of %d Subsystems    %d of %d Components   %d total lines of code",
		system,
		len(components),
		len(components),
		0,
		0,
		len(components),
		len(components),
		loc,
	)
	return header, rows
}

func (mv *MainView) detailContents(components []models.Component, selected int) string {
	return fmt.Sprintf("Description: %s", components[selected].Description)
}

func (mv *MainView) showToggleContents() string {
	contents := ""
	keys := map2slice(mv.show)
	for _, key := range keys {
		mark := " "
		if mv.show[key] {
			mark = "x"
		}
		contents += fmt.Sprintf("[%s] %s\n", mark, key)
	}
	return contents
}

func (mv *MainView) InitKeyBindings(g *gocui.Gui) error {
	return mv.setKeyBindings(ItemsView, g)
}

func (mv *MainView) setKeyBindings(view string, g *gocui.Gui) error {
	if view != ItemsView && view != ShowView {
		return nil
	}
	err := g.SetKeybinding(view, 'k', gocui.ModNone, mv.cursorUp)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(view, 'j', gocui.ModNone, mv.cursorDown)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(view, 'g', gocui.ModNone, mv.topOfList)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(view, 'G', gocui.ModNone, mv.bottomOfList)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(view, 's', gocui.ModNone, mv.toggleShowToggle)
	if err != nil {
		return err
	}
	return nil
}

func (mv *MainView) toggleShowToggle(g *gocui.Gui, v *gocui.View) error {
	switch mv.mode {
	case MainViewModeListBar:
		mv.mode = MainViewModeToggleShow
	case MainViewModeToggleShow:
		mv.mode = MainViewModeListBar
	}
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

func (mv *MainView) topOfList(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	ox, oy := v.Origin()
	cx, _ := v.Cursor()
	if err := v.SetCursor(cx, 0); err != nil && oy > 0 {
		if err := v.SetOrigin(ox, 0); err != nil {
			return err
		}
	}
	return nil
}

func (mv *MainView) bottomOfList(g *gocui.Gui, v *gocui.View) error {
	if v == nil {
		return nil
	}
	lines := len(mv.graph.Components(mv.subsystem))
	ox, oy := v.Origin()
	cx, _ := v.Cursor()
	if err := v.SetCursor(cx, lines-100); err != nil && oy > 0 {
		if err := v.SetOrigin(ox, lines-1); err != nil {
			return err
		}
	}
	return nil
}
