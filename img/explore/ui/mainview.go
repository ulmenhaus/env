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

	components []models.Component // the current list of components being shown in the UI
	kind2count map[string]int     // breakdown of the counts of components under the current subsystem
	mode       MainViewMode
}

// NewMainView returns a MainView initialized with a given Table
func NewMainView(graph *models.SystemGraph, g *gocui.Gui) (*MainView, error) {
	mv := &MainView{
		graph:     graph,
		show:      map[string]bool{},
		subsystem: models.RootSystem,
	}
	for _, comp := range graph.Components(mv.subsystem) {
		mv.show[comp.Kind] = false
	}
	mv.show["struct"] = true
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
		if err == gocui.ErrUnknownView {
			err = mv.setKeyBindings(ShowView, g)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		showToggle.Clear()
		showToggleContents := mv.showToggleContents()
		fmt.Fprintf(showToggle, showToggleContents)
		showToggle.SelBgColor = gocui.ColorWhite
		showToggle.SelFgColor = gocui.ColorBlack
		showToggle.Highlight = true
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

	allComponents := mv.graph.Components(mv.subsystem)
	mv.kind2count = breakdown(allComponents)
	mv.components = mv.filterComponents(allComponents)
	headerS, rows := mv.tabulatedItems(mv.components)
	for _, row := range rows {
		fmt.Fprintf(items, "%s\n", row)
	}
	fmt.Fprintf(header, headerS)
	_, oy := items.Origin()
	_, cy := items.Cursor()
	detailContents := mv.detailContents(mv.components, oy+cy)
	fmt.Fprintf(detail, detailContents)

	return nil
}

func breakdown(c []models.Component) map[string]int {
	m := map[string]int{}
	for _, component := range c {
		m[component.Kind] += 1
	}
	return m
}

func (mv *MainView) filterComponents(c []models.Component) []models.Component {
	filtered := []models.Component{}
	for _, component := range c {
		if mv.show[component.Kind] {
			filtered = append(filtered, component)
		}
	}
	return filtered
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
	if selected >= len(components) {
		return ""
	}
	if components[selected].Description == "" {
		return "No Documentation Provided"
	}
	return fmt.Sprintf("Description: %s", components[selected].Description)
}

func (mv *MainView) showToggleContents() string {
	contents := ""
	keys := map2slice(mv.show)

	padding := 10
	width := 0

	for _, key := range keys {
		if len(key) > width {
			width = len(key)
		}
	}

	for _, key := range keys {
		mark := " "
		if mv.show[key] {
			mark = "x"
		}
		spacing := stringMult(" ", width - len(key) + padding)
		contents += fmt.Sprintf("[%s] %s%s%d\n", mark, key, spacing, mv.kind2count[key])
	}
	return contents
}

func (mv *MainView) toggleToggle(g *gocui.Gui, v *gocui.View) error {
	keys := map2slice(mv.show)
	_, oy := v.Origin()
	_, cy := v.Cursor()
	key := keys[oy+cy]
	mv.show[key] = !mv.show[key]
	return nil
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
	err = g.SetKeybinding(view, 'c', gocui.ModNone, mv.toggleCurrentEntry)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(view, gocui.KeyEnter, gocui.ModNone, mv.enterSubsystem)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(view, 'u', gocui.ModNone, mv.exitSubsystem)
	if err != nil {
		return err
	}
	err = g.SetKeybinding(view, gocui.KeySpace, gocui.ModNone, mv.toggleToggle)
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

func (mv *MainView) toggleCurrentEntry(g *gocui.Gui, v *gocui.View) error {
	_, oy := v.Origin()
	_, cy := v.Cursor()
	uid := mv.components[oy+cy].UID
	uncontained := mv.graph.Uncontained(uid, mv.subsystem)
	// If there are entries to move then we will move them. Otherwise
	// we will expand the subsystem instead.
	if len(uncontained) > 0 {
		for _, tomove := range uncontained {
			mv.graph.MoveInto(tomove, uid)
		}
	} else {
		for _, tomove := range mv.graph.Components(uid) {
			mv.graph.MoveInto(tomove.UID, mv.subsystem)
		}
	}
	return nil
}

func (mv *MainView) enterSubsystem(g *gocui.Gui, v *gocui.View) error {
	_, oy := v.Origin()
	_, cy := v.Cursor()
	uid := mv.components[oy+cy].UID
	mv.subsystem = uid
	return nil
}

func (mv *MainView) exitSubsystem(g *gocui.Gui, v *gocui.View) error {
	mv.subsystem = mv.graph.Parent(mv.subsystem)
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

func stringMult(s string, count int) string {
	mult := ""
	for i := 0; i < count; i++ {
		mult += s
	}
	return mult
}
