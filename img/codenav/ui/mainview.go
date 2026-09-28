// Package ui implements a tripartite gocui layout for interactively
// navigating a codedb graph: a main table of components in the middle, the
// components it references to the right, and the components that
// reference it to the left.
package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jroimartin/gocui"
	"github.com/ulmenhaus/env/img/codenav/db"
)

// Pane identifies one of the three navigable list panes.
type Pane int

const (
	PaneLeft Pane = iota
	PaneMid
	PaneRight
)

// Mode identifies whether a modal overlay currently owns input -- the help
// screen, the type-filter dialog, or the search box -- or none does.
type Mode int

const (
	ModeNormal Mode = iota
	ModeHelp
	ModeTypeFilter
	ModeSearch
)

// ShowMode controls what the mid pane shows relative to the current root:
// only its direct children, or its full set of descendants.
type ShowMode int

const (
	// ShowDescendants is the zero value so a fresh MainView -- root "" --
	// starts out showing every component, matching the tool's original,
	// root-less behavior.
	ShowDescendants ShowMode = iota
	ShowChildren
)

// ColsMode controls which set of mid-table columns (besides the always-
// present Type and Name) is currently shown.
type ColsMode int

const (
	// ColsComponents is the zero value: LoC, CH, DE, PU, PV, TE, matching
	// the columns the tool originally showed (plus PU/PV/TE).
	ColsComponents ColsMode = iota
	ColsConnectors          // RI, RO, RW
)

// midColumn identifies one of the mid pane's table columns, in display
// order.
type midColumn int

const (
	ColType midColumn = iota
	ColName
	ColLoC
	ColCH
	ColDE
	ColPU
	ColPV
	ColTE
	ColRI
	ColRO
	ColRW
	ColLG
	numMidColumns
)

func (c midColumn) label() string {
	switch c {
	case ColType:
		return "Type"
	case ColName:
		return "Name"
	case ColLoC:
		return "LoC"
	case ColCH:
		return "CH"
	case ColDE:
		return "DE"
	case ColPU:
		return "PU"
	case ColPV:
		return "PV"
	case ColTE:
		return "TE"
	case ColRI:
		return "RI"
	case ColRO:
		return "RO"
	case ColRW:
		return "RW"
	case ColLG:
		return "LG"
	default:
		return ""
	}
}

const (
	BreadcrumbsView = "breadcrumbs"

	LeftSummaryView = "left-summary"
	LeftHeaderView  = "left-header"
	LeftListView    = "left-list"

	MidSummaryView = "mid-summary"
	MidHeaderView  = "mid-header"
	MidListView    = "mid-list"

	RightSummaryView = "right-summary"
	RightHeaderView  = "right-header"
	RightListView    = "right-list"

	HelpView       = "help"
	TypeFilterView = "type-filter"
	SearchView     = "search"

	breadcrumbsHeight = 3
	// summaryHeight covers the side panes' three summary lines (the
	// count/current-selection line, their own search query, and their own
	// type filter).
	summaryHeight   = 5
	headerHeight    = 3
	searchBarHeight = 3

	// midSummaryHeight is taller than the side panes' summaryHeight: the
	// mid pane's summary carries six lines (root path, search query, type
	// filter, show mode, cols mode, component count) instead of one.
	midSummaryHeight = 8

	// Fixed widths for the mid pane's table columns; Name gets whatever
	// space is left over, computed at layout time. Which of these are
	// actually shown depends on colsMode -- see midColumnLayout.
	widthType = 10
	widthLoC  = 6
	widthCH   = 5
	widthDE   = 6
	widthPU   = 6
	widthPV   = 6
	widthTE   = 6
	widthRI   = 5
	widthRO   = 5
	widthRW   = 5
	widthLG   = 4

	// widthRefCount is the fixed width of the side panes' RT/RF column.
	widthRefCount = 5

	// widthLevel is the fixed width of the Referenced By pane's LV
	// column (see layoutColumn's showLevel).
	widthLevel = 4

	// FocusColor is the frame color gocui uses to highlight whichever pane
	// is current (see g.Highlight / g.SelFgColor set up in main.go).
	FocusColor = gocui.ColorGreen
)

// MainView is the root of the codedb navigator's UI. It owns the loaded
// database and tracks which component is currently selected in the middle
// (exploring) pane, along with the pre-computed adjacency lists shown in
// the side panes.
type MainView struct {
	dbPath string
	graph  *db.DB

	// g is set on the first Layout call so helpers like cursorIndex can
	// look up view state outside of a keybinding handler (which is always
	// handed a *gocui.Gui directly).
	g *gocui.Gui

	focus Pane

	// root is the component the mid pane is currently scoped under; ""
	// means unscoped (every component is in play). showMode says whether
	// that scope is root's direct children or its full descendant set
	// (see updateRootMembers). rootConstrained/rootMembers cache the
	// resulting base list -- recomputed only when root or showMode
	// changes -- so applyFilters doesn't have to re-walk the tree on
	// every keystroke of a search or every sort.
	root            string
	showMode        ShowMode
	rootConstrained bool
	rootMembers     []string

	// names is the list actually shown in the mid pane: rootMembers (or,
	// if unconstrained, every component) minus whatever the type filter
	// and search query exclude, in the order the current sort leaves it
	// (see applyFilters). All cursor/row-index math in the mid pane
	// operates on names.
	names []string

	// colsMode picks which set of columns (besides Type/Name) the mid
	// table shows -- toggled with c (see toggleColsMode).
	colsMode ColsMode

	// selectedCol is the mid table column currently selected for l/h
	// navigation, o/O sorting, and the f/"/" filter shortcuts. Always one
	// of the columns colsMode currently makes visible (see
	// toggleColsMode, colLeft/colRight, activeColumns).
	selectedCol midColumn
	// sortCol/sortAsc record the column and direction names is currently
	// sorted by, purely to render a sort indicator in the header; sortCol
	// is -1 when no sort has been applied yet (names is in the database's
	// default, alphabetical-by-name order).
	sortCol midColumn
	sortAsc bool

	// typeFilter says which Kind values are currently checked in the mid
	// pane's type filter dialog (see layoutTypeFilter); a component whose
	// Kind maps to false is excluded from names. Every kind starts
	// checked. leftTypeFilter/rightTypeFilter are the same idea for the
	// References/Referenced By panes: they exclude entries from
	// filteredInbound/filteredOutbound (see refreshSideFilters) by the
	// entry's own Kind, AND restrict which edges count in the mid table's
	// RefsIn/RefsOut/RefsWithin (see recomputeRefCounts), so those numbers
	// stay in sync with what the panes actually show. Which of the three
	// a keypress operates on is dispatched by focus -- see
	// activeTypeFilter.
	typeFilter      map[string]bool
	leftTypeFilter  map[string]bool
	rightTypeFilter map[string]bool
	// searchQuery is the live text typed into the mid pane's search box; a
	// component is excluded from names unless its LowerName contains it.
	// Empty means no search filter is active. Shown in the mid summary
	// (see searchRowText) and reset -- along with leftSearchQuery and
	// rightSearchQuery, see resetAllSearchQueries -- on every root change
	// (setRoot, goUpRoot, goToTopRoot), not just when explicitly cleared
	// via ESC.
	searchQuery string
	// leftSearchQuery/rightSearchQuery are the References/Referenced By
	// panes' own, independent search boxes (opened with / while one of
	// those panes has focus -- see openSearch/Edit, which dispatch on
	// focus). They filter filteredInbound/filteredOutbound, not names, and
	// are shown in their pane's own summary (see layoutColumn).
	leftSearchQuery  string
	rightSearchQuery string

	// current is the component selected in the mid pane. accumOut/accumIn
	// are its references rolled up through its whole containment subtree
	// (see db.AccumulatedReferences): accumOut[X] / accumIn[X] is the
	// summed count of direct edges between X and any descendant-or-self of
	// current. outbound/inbound are just those maps' sorted keys, for
	// stable list order; filteredOutbound/filteredInbound are what the
	// side panes actually display -- outbound/inbound further narrowed by
	// rightSearchQuery/leftSearchQuery (see refreshSideFilters). (The mid
	// table's own RI/RO/RW columns don't need any of this -- they're
	// precomputed for every component at load time; see db.Component.)
	// Recomputed only when the mid selection or a side query changes.
	current          string
	accumOut         map[string]int
	accumIn          map[string]int
	outbound         []string
	inbound          []string
	filteredOutbound []string
	filteredInbound  []string

	// mode is which modal overlay, if any, currently owns input.
	mode Mode

	// dirty tracks which panes need their buffers repopulated on the next
	// Layout call. Rewriting a pane's content is O(len(list)), so with
	// graphs in the hundreds of thousands of components we only want to
	// pay that cost when the underlying list actually changed -- not on
	// every keypress-triggered redraw. The mid header is cheap (a single
	// line) and is simply redrawn fresh every Layout call instead.
	dirty map[string]bool
}

// allKindsChecked returns a fresh type-filter map with every one of
// graph's Kinds checked (true) -- the "show everything" starting point for
// typeFilter/leftTypeFilter/rightTypeFilter, also used to reset them (see
// resetAllTypeFilters).
func allKindsChecked(graph *db.DB) map[string]bool {
	tf := make(map[string]bool, len(graph.Kinds))
	for _, kind := range graph.Kinds {
		tf[kind] = true
	}
	return tf
}

// NewMainView loads the codedb graph at dbPath and returns a MainView ready
// to be driven by gocui.
func NewMainView(dbPath string) (*MainView, error) {
	graph, err := db.Load(dbPath)
	if err != nil {
		return nil, err
	}
	mv := &MainView{
		dbPath:          dbPath,
		graph:           graph,
		focus:           PaneMid,
		selectedCol:     ColType,
		sortCol:         -1,
		typeFilter:      allKindsChecked(graph),
		leftTypeFilter:  allKindsChecked(graph),
		rightTypeFilter: allKindsChecked(graph),
		dirty: map[string]bool{
			MidListView:   true,
			LeftListView:  true,
			RightListView: true,
		},
	}
	mv.updateRootMembers()
	mv.applyFilters()
	return mv, nil
}

// applyFilters rebuilds names from allNames, keeping only components whose
// Kind is checked in typeFilter and whose LowerName contains searchQuery.
// It's O(len(allNames)) -- the unavoidable cost of a live substring filter
// -- so it only runs on an explicit filter/search/sort change, never on
// plain j/k/PgUp/PgDn navigation. It always jumps the mid selection back to
// the top of the new list, same as a sort does.
func (mv *MainView) applyFilters() {
	base := mv.graph.Names
	if mv.rootConstrained {
		base = mv.rootMembers
	}

	query := strings.ToLower(mv.searchQuery)
	filtered := make([]string, 0, len(base))
	for _, name := range base {
		c := mv.graph.Components[name]
		if !mv.typeFilter[c.Kind] {
			continue
		}
		if query != "" && !strings.Contains(c.LowerName, query) {
			continue
		}
		filtered = append(filtered, name)
	}
	// base is already alphabetically sorted (graph.Names, or
	// DirectChildren/DescendantNames -- see updateRootMembers), and
	// filtering preserves order, so an explicit sort is only needed when
	// the user picked a different column to sort by.
	if mv.sortCol >= 0 {
		sortComponents(mv.graph, filtered, mv.sortCol, mv.sortAsc)
	}

	mv.names = filtered
	mv.dirty[MidListView] = true
	if mv.g != nil {
		if v, err := mv.g.View(MidListView); err == nil {
			_ = v.SetCursor(0, 0)
			_ = v.SetOrigin(0, 0)
		}
	}
	mv.refreshSelection()
}

// updateRootMembers recomputes the base set the mid pane draws from,
// given the current root and showMode. It's the only place that walks the
// tree for this purpose, called just when root or showMode actually
// changes -- not on every applyFilters call.
func (mv *MainView) updateRootMembers() {
	if mv.showMode == ShowChildren {
		mv.rootMembers = mv.graph.DirectChildren(mv.root)
		mv.rootConstrained = true
		return
	}
	if mv.root == "" {
		mv.rootConstrained = false
		mv.rootMembers = nil
		return
	}
	mv.rootMembers = mv.graph.DescendantNames(mv.root)
	mv.rootConstrained = true
}

// setRoot descends into name: it becomes the new root, the mid pane
// refocuses to show its children/descendants (per the current showMode),
// and focus moves to the mid pane since that's what just changed.
func (mv *MainView) setRoot(name string) {
	if name == "" || name == mv.root {
		return
	}
	mv.root = name
	mv.resetAllSearchQueries()
	mv.resetAllTypeFilters()
	mv.updateRootMembers()
	mv.applyFilters()
	mv.focus = PaneMid
}

// goUpRoot takes one component off the end of root (root becomes its own
// parent, possibly "") and then selects the old root in the resulting mid
// list, so the view lands back where you were before you drilled in.
func (mv *MainView) goUpRoot() {
	if mv.root == "" {
		return
	}
	prev := mv.root
	mv.root = mv.graph.Components[mv.root].SoParent
	mv.resetAllSearchQueries()
	mv.resetAllTypeFilters()
	mv.updateRootMembers()
	mv.applyFilters()
	mv.focus = PaneMid
	mv.selectByName(prev)
}

// goToTopRoot jumps straight to root "" -- showing everything (in
// descendants mode) or every top-level component (in children mode).
func (mv *MainView) goToTopRoot() {
	if mv.root == "" {
		return
	}
	mv.root = ""
	mv.resetAllSearchQueries()
	mv.resetAllTypeFilters()
	mv.updateRootMembers()
	mv.applyFilters()
	mv.focus = PaneMid
}

// selectByName scrolls and moves the mid list's cursor to name, if it's
// present in the current names, roughly centering it in the viewport.
func (mv *MainView) selectByName(name string) {
	idx := -1
	for i, n := range mv.names {
		if n == name {
			idx = i
			break
		}
	}
	if idx < 0 || mv.g == nil {
		return
	}
	v, err := mv.g.View(MidListView)
	if err != nil {
		return
	}
	_, height := v.Size()
	if height < 1 {
		height = 1
	}
	maxOrigin := len(mv.names) - height
	if maxOrigin < 0 {
		maxOrigin = 0
	}
	origin := idx - height/2
	if origin < 0 {
		origin = 0
	}
	if origin > maxOrigin {
		origin = maxOrigin
	}
	_ = v.SetOrigin(0, origin)
	_ = v.SetCursor(0, idx-origin)
	mv.refreshSelection()
}

// refreshSelection recomputes the currently selected mid-pane component and
// its adjacency lists. It should be called whenever the mid pane's cursor
// moves to a new row, or the row at the current cursor position changes
// out from under it (e.g. after a sort).
func (mv *MainView) refreshSelection() {
	current := ""
	if len(mv.names) > 0 {
		idx := mv.midIndex()
		if idx < 0 {
			idx = 0
		}
		if idx >= len(mv.names) {
			idx = len(mv.names) - 1
		}
		current = mv.names[idx]
	}
	if current == mv.current {
		return
	}
	mv.current = current
	mv.refreshAccumulatedReferences()
	// The mid table's row marker (see formatMidRow) is only drawn on the
	// current row, so a full repaint is needed whenever which row that is
	// changes -- i.e. on every j/k press, not just h/l or a sort. That
	// makes j/k an O(len(names)) repaint rather than the O(1) cursor move
	// it was before; see the note on colLeft for why that trade was made.
	mv.dirty[MidListView] = true
}

// refreshAccumulatedReferences recomputes accumOut/accumIn -- and their
// sorted-key views, outbound/inbound -- for the current selection. "" (no
// selection, an empty mid list) short-circuits to empty rather than
// calling AccumulatedReferences(""), which would misinterpret "" as the
// forest's own root and walk every top-level component (see
// db.DirectChildren's "" sentinel).
//
// leftTypeFilter/rightTypeFilter are passed straight into
// AccumulatedReferences, which rolls a reference up to whichever ancestor
// of the far endpoint they allow rather than excluding it outright (see
// its doc comment) -- so this must be called not only when the mid
// selection changes (see refreshSelection) but also whenever either side
// filter does (see filterToggle), since the roll-up itself depends on it.
func (mv *MainView) refreshAccumulatedReferences() {
	if mv.current == "" {
		mv.accumOut, mv.accumIn = map[string]int{}, map[string]int{}
	} else {
		mv.accumOut, mv.accumIn = mv.graph.AccumulatedReferences(mv.current, mv.leftTypeFilter, mv.rightTypeFilter)
	}
	mv.outbound = sortedKeys(mv.accumOut)
	mv.inbound = sortedKeys(mv.accumIn)
	mv.refreshSideFilters()
}

// refreshSideFilters rebuilds filteredInbound/filteredOutbound from
// inbound/outbound and the two side panes' own search queries. Type
// filtering already happened in AccumulatedReferences (see
// refreshAccumulatedReferences) -- every name in inbound/outbound is
// already guaranteed to pass its pane's type filter by construction, so
// this only narrows by search query. Called whenever inbound/outbound
// themselves change or either side's search query changes (see
// setActiveSearchQuery).
func (mv *MainView) refreshSideFilters() {
	mv.filteredInbound = filterByQuery(mv.inbound, mv.leftSearchQuery)
	mv.filteredOutbound = filterByQuery(mv.outbound, mv.rightSearchQuery)
	mv.dirty[LeftListView] = true
	mv.dirty[RightListView] = true
}

// filterByQuery narrows items (component names) to those containing query
// as a case-insensitive substring; an empty query matches everything.
func filterByQuery(items []string, query string) []string {
	if query == "" {
		return items
	}
	q := strings.ToLower(query)
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item), q) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// activeSearchQuery and setActiveSearchQuery dispatch to whichever of the
// three search queries belongs to the currently focused pane -- the mid
// pane's own searchQuery, or one of the side panes' -- since focus is what
// determined which one "/" opened (see openSearch) and stays put for the
// duration of ModeSearch even though the search box itself becomes the
// current view.
func (mv *MainView) activeSearchQuery() string {
	switch mv.focus {
	case PaneLeft:
		return mv.leftSearchQuery
	case PaneRight:
		return mv.rightSearchQuery
	default:
		return mv.searchQuery
	}
}

func (mv *MainView) setActiveSearchQuery(q string) {
	switch mv.focus {
	case PaneLeft:
		mv.leftSearchQuery = q
		mv.refreshSideFilters()
	case PaneRight:
		mv.rightSearchQuery = q
		mv.refreshSideFilters()
	default:
		mv.searchQuery = q
		mv.applyFilters()
	}
}

// resetAllSearchQueries clears all three search queries and refreshes the
// side panes' filtered lists accordingly. It does NOT re-apply the mid
// pane's own filters (mv.names) -- callers that also need that (root
// changes, and closing the mid search box) call applyFilters themselves
// right alongside it.
func (mv *MainView) resetAllSearchQueries() {
	mv.searchQuery = ""
	mv.leftSearchQuery = ""
	mv.rightSearchQuery = ""
	mv.refreshSideFilters()
}

// activeTypeFilter dispatches to whichever of the three type-filter maps
// belongs to the currently focused pane, the same way activeSearchQuery
// does for search queries. Unlike search queries these are maps, so
// there's no separate "set" counterpart -- callers just mutate the
// returned map in place (see filterToggle) and then call the appropriate
// refresh themselves.
func (mv *MainView) activeTypeFilter() map[string]bool {
	switch mv.focus {
	case PaneLeft:
		return mv.leftTypeFilter
	case PaneRight:
		return mv.rightTypeFilter
	default:
		return mv.typeFilter
	}
}

// recomputeRefCounts redoes the mid table's RefsIn/RefsOut/RefsWithin
// (see db.RecomputeAccumulatedRefCounts) restricted to sources checked in
// leftTypeFilter and dests checked in rightTypeFilter -- the same two
// filters that decide what the References (RT, sources) and Referenced By
// (RF, dests) panes display, so these numbers stay in sync with those
// panes. It's an O(edges) walk, same order of cost as the original
// load-time computation, so it's only called on an explicit type-filter
// change (see filterToggle, resetAllTypeFilters), never per keystroke.
func (mv *MainView) recomputeRefCounts() {
	mv.graph.RecomputeAccumulatedRefCounts(mv.leftTypeFilter, mv.rightTypeFilter)
	mv.dirty[MidListView] = true
}

// resetAllTypeFilters checks every Kind in all three type filters (undoing
// any filtering in the mid pane and both side panes) and recomputes the
// mid table's reference counts accordingly. Called on every root change
// (setRoot, goUpRoot, goToTopRoot) alongside resetAllSearchQueries, on the
// same "start fresh" rationale.
func (mv *MainView) resetAllTypeFilters() {
	mv.typeFilter = allKindsChecked(mv.graph)
	mv.leftTypeFilter = allKindsChecked(mv.graph)
	mv.rightTypeFilter = allKindsChecked(mv.graph)
	mv.graph.RecomputeAccumulatedRefCounts(nil, nil)
	mv.dirty[MidListView] = true
	mv.refreshAccumulatedReferences()
}

func (mv *MainView) midIndex() int {
	return mv.cursorIndex(MidListView)
}

// cursorIndex looks up the absolute row a view's cursor is on. It degrades
// to 0 before the view exists (e.g. during the very first Layout pass).
func (mv *MainView) cursorIndex(name string) int {
	if mv.g == nil {
		return 0
	}
	v, err := mv.g.View(name)
	if err != nil {
		return 0
	}
	_, cy := v.Cursor()
	_, oy := v.Origin()
	return cy + oy
}

func (mv *MainView) Layout(g *gocui.Gui) error {
	mv.g = g
	maxX, maxY := g.Size()
	minHeight := breadcrumbsHeight + midSummaryHeight + headerHeight + 2
	if mv.mode == ModeSearch {
		minHeight += searchBarHeight
	}
	if maxX < 3 || maxY < minHeight {
		// Terminal too small to lay anything out sensibly; wait for a
		// resize rather than erroring out.
		return nil
	}

	if err := mv.layoutBreadcrumbs(g, maxX); err != nil {
		return err
	}

	// The search box is a persistent bottom bar, so it steals space from
	// the three panes above it; the help and type-filter dialogs are
	// floating overlays drawn on top instead, so they don't affect this.
	contentMaxY := maxY
	if mv.mode == ModeSearch {
		contentMaxY -= searchBarHeight
	}

	leftX1 := maxX / 4
	rightX0 := maxX - maxX/4

	if err := mv.layoutColumn(g, PaneLeft, 0, leftX1, maxX, contentMaxY, "References", "RT", mv.filteredInbound, mv.leftSearchQuery, mv.leftTypeFilter,
		func(item string) int { return mv.accumIn[item] }, false); err != nil {
		return err
	}
	if err := mv.layoutMidTable(g, leftX1+1, rightX0-1, maxX, contentMaxY); err != nil {
		return err
	}
	if err := mv.layoutColumn(g, PaneRight, rightX0, maxX-1, maxX, contentMaxY, "Referenced By", "RF", mv.filteredOutbound, mv.rightSearchQuery, mv.rightTypeFilter,
		func(item string) int { return mv.accumOut[item] }, true); err != nil {
		return err
	}

	// Only one overlay/modal view exists at a time; tear down whichever
	// ones aren't for the current mode before (re)drawing it.
	for _, name := range []string{HelpView, TypeFilterView, SearchView} {
		active := (mv.mode == ModeHelp && name == HelpView) ||
			(mv.mode == ModeTypeFilter && name == TypeFilterView) ||
			(mv.mode == ModeSearch && name == SearchView)
		if !active {
			if err := g.DeleteView(name); err != nil && err != gocui.ErrUnknownView {
				return err
			}
		}
	}

	switch mv.mode {
	case ModeHelp:
		if err := mv.layoutHelp(g, maxX, maxY); err != nil {
			return err
		}
		_, err := g.SetCurrentView(HelpView)
		return err
	case ModeTypeFilter:
		if err := mv.layoutTypeFilter(g, maxX, maxY); err != nil {
			return err
		}
		_, err := g.SetCurrentView(TypeFilterView)
		return err
	case ModeSearch:
		if err := mv.layoutSearchBar(g, maxX, contentMaxY, maxY); err != nil {
			return err
		}
		_, err := g.SetCurrentView(SearchView)
		return err
	}

	_, err := g.SetCurrentView(mv.listViewName(mv.focus))
	return err
}

func (mv *MainView) layoutBreadcrumbs(g *gocui.Gui, maxX int) error {
	v, err := g.SetView(BreadcrumbsView, 0, 0, maxX-1, breadcrumbsHeight-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	v.Frame = true
	v.Clear()
	fmt.Fprintf(v, " %s", mv.dbPath)
	return nil
}

// layoutColumn draws one of the two side (summary, header, list) column
// triples -- References or Referenced By -- as a table: the item's name,
// countLabel ("RT" or "RF", the number of individual base_references
// directly between it and the mid pane's selected component, from
// countFor), and, only when showLevel is true (the Referenced By pane),
// a trailing LV column giving that item's own RootLevel (see Component).
// items is expected to already be filtered by query (see
// refreshSideFilters) -- query is only used here to render the pane's own
// "Search: ..." line. The list is only repopulated when its pane is
// marked dirty, which happens whenever the selected component or this
// pane's own search query changes (see refreshSelection /
// refreshSideFilters) -- precisely when items/counts would change too.
func (mv *MainView) layoutColumn(g *gocui.Gui, pane Pane, x0, x1, maxX, maxY int, label, countLabel string, items []string, query string, typeFilter map[string]bool, countFor func(string) int, showLevel bool) error {
	summaryName, headerName, listName := mv.summaryViewName(pane), mv.headerViewName(pane), mv.listViewName(pane)

	summaryY0 := breadcrumbsHeight
	summaryY1 := summaryY0 + summaryHeight - 1
	headerY0 := summaryY1 + 1
	headerY1 := headerY0 + headerHeight - 1
	listY0 := headerY1 + 1
	listY1 := maxY - 1

	summary, err := g.SetView(summaryName, x0, summaryY0, x1, summaryY1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	summary.Frame = true
	summary.Clear()
	title := label
	if mv.current == "" {
		title = fmt.Sprintf("%s (select a component)", label)
	} else {
		title = fmt.Sprintf("%s: %s", label, mv.current)
	}
	fmt.Fprintln(summary, fmt.Sprintf(" %s (%d)", title, len(items)))
	searchLine := "Search: (none)"
	if query != "" {
		searchLine = "Search: " + query
	}
	fmt.Fprintln(summary, " "+searchLine)
	fmt.Fprintf(summary, " %s", mv.typesRowText(typeFilter))

	extra := widthRefCount + 1
	if showLevel {
		extra += widthLevel + 1
	}
	nameWidth := (x1 - x0 - 1) - extra
	if nameWidth < 10 {
		nameWidth = 10
	}

	header, err := g.SetView(headerName, x0, headerY0, x1, headerY1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	header.Frame = true
	header.Clear()
	if showLevel {
		fmt.Fprintf(header, "%-*s %*s %*s", nameWidth, "Name", widthRefCount, countLabel, widthLevel, "LV")
	} else {
		fmt.Fprintf(header, "%-*s %*s", nameWidth, "Name", widthRefCount, countLabel)
	}

	list, err := g.SetView(listName, x0, listY0, x1, listY1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	list.Frame = true
	list.Highlight = true
	list.SelBgColor = gocui.ColorWhite
	list.SelFgColor = gocui.ColorBlack
	// The focused pane's border is highlighted by gocui itself: it draws
	// whichever view is g.currentView with g.SelFgColor/SelBgColor (see
	// the g.Highlight setup in main.go), so all that's needed here is to
	// make sure the focused pane becomes the current view (done at the
	// end of Layout).

	if mv.dirty[listName] {
		list.Clear()
		for _, item := range items {
			if showLevel {
				fmt.Fprintf(list, "%-*s %*d %*d\n", nameWidth, truncate(item, nameWidth), widthRefCount, countFor(item), widthLevel, mv.graph.Components[item].RootLevel)
			} else {
				fmt.Fprintf(list, "%-*s %*d\n", nameWidth, truncate(item, nameWidth), widthRefCount, countFor(item))
			}
		}
		// Content changed out from under this pane, so reset the cursor
		// rather than risk it pointing past the new list's end.
		_ = list.SetCursor(0, 0)
		_ = list.SetOrigin(0, 0)
		mv.dirty[listName] = false
	}

	return nil
}

// layoutMidTable draws the mid pane as three stacked boxes: a summary, a
// column header (Type, Name, LoC, CH, DE, RI, RO -- with the selected
// column highlighted and, if names is sorted, a direction arrow), and the
// scrolling table of components itself. The header is cheap (a handful of
// short strings) so it's redrawn fresh every call; the data rows are only
// rewritten when dirty, since that cost is O(len(names)).
func (mv *MainView) layoutMidTable(g *gocui.Gui, x0, x1, maxX, maxY int) error {
	summaryY0 := breadcrumbsHeight
	summaryY1 := summaryY0 + midSummaryHeight - 1
	headerY0 := summaryY1 + 1
	headerY1 := headerY0 + headerHeight - 1
	listY0 := headerY1 + 1
	listY1 := maxY - 1

	summary, err := g.SetView(MidSummaryView, x0, summaryY0, x1, summaryY1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	summary.Frame = true
	summary.Clear()
	fmt.Fprintln(summary, " "+mv.rootPathText(x1-x0-2))
	fmt.Fprintln(summary, " "+mv.searchRowText())
	fmt.Fprintln(summary, " "+mv.typesRowText(mv.typeFilter))
	fmt.Fprintln(summary, " "+mv.modeRowText())
	fmt.Fprintln(summary, " "+mv.colsRowText())
	fmt.Fprintf(summary, " Components (%d)", len(mv.names))

	nameWidth := (x1 - x0 - 1) - mv.midFixedColsWidth()
	if nameWidth < 10 {
		nameWidth = 10
	}

	header, err := g.SetView(MidHeaderView, x0, headerY0, x1, headerY1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	header.Frame = true
	header.Clear()
	fmt.Fprint(header, mv.formatMidHeaderLine(nameWidth))

	list, err := g.SetView(MidListView, x0, listY0, x1, listY1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	list.Frame = true
	list.Highlight = true
	list.SelBgColor = gocui.ColorWhite
	list.SelFgColor = gocui.ColorBlack

	if mv.dirty[MidListView] {
		// Clear() doesn't touch the cursor/origin, so a repaint triggered
		// merely by moving the selected column (h/l) leaves the user's
		// scroll position alone. A repaint triggered by an actual reorder
		// (sort) does jump to the top -- but that's because applySort
		// explicitly resets the view's cursor/origin itself before
		// marking this dirty, not anything done here.
		list.Clear()
		for _, name := range mv.names {
			fmt.Fprintln(list, mv.formatMidRow(nameWidth, mv.graph.Components[name], name == mv.current))
		}
		mv.dirty[MidListView] = false
	}

	return nil
}

// layoutHelp draws a centered overlay summarizing the mid table's columns
// and how to sort by them.
func (mv *MainView) layoutHelp(g *gocui.Gui, maxX, maxY int) error {
	lines := []string{
		"Columns (middle pane)",
		"",
		"  Type and Name are always shown. c toggles which other columns",
		"  show -- Components or Connectors (see the Cols row above the",
		"  table, and Cols mode below).",
		"",
		"  Components cols:",
		"  LoC    lines of code (this component plus all its descendants)",
		"  CH     children -- direct components whose parent is this one",
		"  DE     descendants -- components recursively contained under this one",
		"  PU     public components in this subtree (this one plus its",
		"         descendants) -- see Categories below",
		"  PV     private components in this subtree",
		"  TE     test components in this subtree",
		"",
		"  Connectors cols:",
		"  RI     references in -- rolled up through this component's whole",
		"         subtree, excluding anything counted in RW (see below)",
		"  RO     references out -- same, rolled up, excluding RW",
		"  RW     references within -- references where both ends are under",
		"         this component; not counted in RI or RO",
		"  LG     level gap -- highest RootLevel minus lowest RootLevel among",
		"         the components counted in RO, 0 if RO is 0 -- see Levels",
		"         below",
		"",
		"Categories",
		"",
		"  Every component is exactly one of: test (its Test field is",
		"  \"yes\"), public (not test, and its Top field is \"yes\"), or",
		"  private (neither). PU/PV/TE count these across a subtree.",
		"",
		"Columns (side panes)",
		"",
		"  RT     references to -- times this row references the selected",
		"         component's subtree (References pane)",
		"  RF     references from -- times the selected component's subtree",
		"         references this row (Referenced By pane)",
		"  LV     (Referenced By pane only) this row's own RootLevel -- see",
		"         Levels below",
		"",
		"Levels",
		"",
		"  Every top-level component (empty SoParent) is a \"package\" for",
		"  this purpose, regardless of its actual Type. A package that",
		"  references no other package is level 1; a package whose only",
		"  package references are to level-1 packages is level 2; and so",
		"  on -- one more than the longest chain of package references",
		"  starting from it. Every component shares its top-level",
		"  ancestor's level as its own RootLevel. Levels are structural --",
		"  computed once at load time from the whole reference graph --",
		"  and don't change with type filters, unlike LG (see above), which",
		"  does since it depends on which references count toward RO.",
		"",
		"Accumulation",
		"",
		"  If a references b, that also counts as every ancestor of a",
		"  referencing every ancestor of b -- UNLESS some ancestor is common",
		"  to both a and b, in which case it counts as a reference within",
		"  that ancestor (RW) instead of crossing its boundary (RI/RO).",
		"  RI/RO/RW are precomputed for every component at load time; the",
		"  side panes' reference lists are computed on demand for whichever",
		"  component is selected, deduplicated and summed by target.",
		"",
		"  RI only counts references whose source passes the References",
		"  pane's type filter; RO only counts references whose dest passes",
		"  the Referenced By pane's type filter -- so RI/RO always match",
		"  what those two panes actually list. Changing either filter",
		"  recomputes RI/RO/RW for every component (an O(edges) pass, same",
		"  cost as the original load-time computation).",
		"",
		"  \"Passes the filter\" means the far endpoint itself OR some",
		"  ancestor of it does -- if a (under selected A) references b",
		"  (under B), and b's own Kind is unchecked but B's isn't, the",
		"  reference still counts, rolled up to B: B appears in the",
		"  Referenced By list (with the count of everything that rolled up",
		"  to it) instead of the entry silently disappearing.",
		"",
		"Sorting",
		"",
		"  h/l       select a column in the middle pane",
		"  o/O       sort by the selected column, ascending / descending",
		"",
		"Filtering & search",
		"",
		"  PgUp/PgDn page through the focused list",
		"  f         open the type filter",
		"  /         open search",
		"",
		"Type filter dialog",
		"",
		"  f also works on the References and Referenced By panes, each",
		"  with its own independent filter -- see Accumulation above for",
		"  how the side panes' filters also affect RI/RO. Every pane's",
		"  summary shows \"Types: n of m\" for its own filter.",
		"",
		"  j/k       move, Enter toggle/select all/deselect all, ESC close",
		"",
		"Search box",
		"",
		"  / also works on the References and Referenced By panes, each",
		"  with its own independent query -- shown in that pane's own",
		"  summary, filtering only what it displays.",
		"",
		"  type to filter components by name as you go",
		"  Enter keep the search and return to the list, ESC clear and close",
		"  clearing the middle pane's search this way also clears both",
		"  side panes' (they're considered part of its search scope)",
		"",
		"Root navigation",
		"",
		"  The mid pane's summary shows the current root's full ancestor",
		"  path, the show mode, and the cols mode (selected one highlighted",
		"  in each).",
		"",
		"  Enter  (any of the 3 lists) make the highlighted item the root",
		"  s      (middle pane) toggle show mode: Children / Descendants",
		"  c      (middle pane) toggle cols mode: Components / Connectors",
		"  q/ESC  go up one root level, reselecting where you came from",
		"  Q      jump straight back to no root (show everything)",
		"",
		"  Enter/q/ESC/Q all also reset every search query and every type",
		"  filter (mid, References, Referenced By) back to showing",
		"  everything -- a new root starts from a clean slate.",
		"",
		"Press ESC to close",
	}
	width := 76
	if width > maxX-4 {
		width = maxX - 4
	}
	height := len(lines) + 2
	if height > maxY-2 {
		height = maxY - 2
	}
	x0, y0 := (maxX-width)/2, (maxY-height)/2

	v, err := g.SetView(HelpView, x0, y0, x0+width, y0+height)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	v.Frame = true
	v.Title = " Help "
	v.Clear()
	for _, line := range lines {
		fmt.Fprintln(v, " "+line)
	}
	return nil
}

// filterRowCount is the number of navigable rows in the type filter
// dialog: the Select All / Deselect All actions, plus one per Kind.
func (mv *MainView) filterRowCount() int {
	return 2 + len(mv.graph.Kinds)
}

// filterRowText renders one row of the type filter dialog: the two bulk
// actions, or a kind's checkbox, name, and how many components have it.
func (mv *MainView) filterRowText(row, width int) string {
	switch row {
	case 0:
		return padCell("[ Select All ]", width, true)
	case 1:
		return padCell("[ Deselect All ]", width, true)
	}
	kind := mv.graph.Kinds[row-2]
	box := "[ ]"
	if mv.activeTypeFilter()[kind] {
		box = "[x]"
	}
	return padCell(fmt.Sprintf("%s %-20s (%d)", box, kind, mv.graph.KindCounts[kind]), width, true)
}

// layoutTypeFilter draws the type-filter dialog: a real gocui list view
// (like the main panes) so its native cursor highlighting shows which row
// is selected, holding the Select All / Deselect All actions followed by
// one checkbox row per Kind. It's tiny -- bounded by the number of
// distinct kinds, not by the graph size -- so it's always redrawn fresh
// rather than dirty-gated.
func (mv *MainView) layoutTypeFilter(g *gocui.Gui, maxX, maxY int) error {
	rows := mv.filterRowCount()
	width := 50
	if width > maxX-4 {
		width = maxX - 4
	}
	height := rows + 2
	if height > maxY-2 {
		height = maxY - 2
	}
	x0, y0 := (maxX-width)/2, (maxY-height)/2

	v, err := g.SetView(TypeFilterView, x0, y0, x0+width, y0+height)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	label := "Components"
	switch mv.focus {
	case PaneLeft:
		label = "References"
	case PaneRight:
		label = "Referenced By"
	}
	v.Frame = true
	v.Title = fmt.Sprintf(" Filter %s by Type (Enter: toggle, ESC: close) ", label)
	v.Highlight = true
	v.SelBgColor = gocui.ColorWhite
	v.SelFgColor = gocui.ColorBlack
	v.Clear()
	innerWidth := width - 1
	for row := 0; row < rows; row++ {
		fmt.Fprintln(v, mv.filterRowText(row, innerWidth))
	}
	return nil
}

// layoutSearchBar draws the persistent search box at the bottom of the
// screen, spanning from y0 to maxY-1, and makes it the Editable current
// view so keystrokes reach Edit (see below) instead of any list binding.
func (mv *MainView) layoutSearchBar(g *gocui.Gui, maxX, y0, maxY int) error {
	v, err := g.SetView(SearchView, 0, y0, maxX-1, maxY-1)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	label := "Name"
	switch mv.focus {
	case PaneLeft:
		label = "References"
	case PaneRight:
		label = "Referenced By"
	}
	v.Frame = true
	v.Title = fmt.Sprintf(" Search (%s) -- Enter: keep, ESC: clear ", label)
	v.Editable = true
	v.Editor = mv
	v.Clear()
	query := mv.activeSearchQuery()
	fmt.Fprintf(v, " %s", query)
	_ = v.SetCursor(len(query)+1, 0)
	return nil
}

func (mv *MainView) summaryViewName(pane Pane) string {
	switch pane {
	case PaneLeft:
		return LeftSummaryView
	case PaneRight:
		return RightSummaryView
	default:
		return MidSummaryView
	}
}

func (mv *MainView) headerViewName(pane Pane) string {
	switch pane {
	case PaneLeft:
		return LeftHeaderView
	case PaneRight:
		return RightHeaderView
	default:
		return MidHeaderView
	}
}

func (mv *MainView) listViewName(pane Pane) string {
	switch pane {
	case PaneLeft:
		return LeftListView
	case PaneRight:
		return RightListView
	default:
		return MidListView
	}
}

// SetKeyBindings wires up j/k navigation within each pane, h/l to move the
// mid table's selected column, o/O to sort by it, space to cycle focus
// between panes, ? to open the help overlay, and ESC to close it.
// keyBinding pairs a view name ("" for global), key, and handler for
// SetKeyBindings' bulk registration below.
type keyBinding struct {
	view string
	key  interface{}
	fn   func(g *gocui.Gui, v *gocui.View) error
}

func (mv *MainView) SetKeyBindings(g *gocui.Gui) error {
	bindings := []keyBinding{
		{MidListView, 'j', mv.midDown},
		{MidListView, 'k', mv.midUp},
		{MidListView, gocui.KeyPgdn, mv.midPageDown},
		{MidListView, gocui.KeyPgup, mv.midPageUp},
		{MidListView, 'h', mv.colLeft},
		{MidListView, 'l', mv.colRight},
		{MidListView, 'o', mv.sortAscending},
		{MidListView, 'O', mv.sortDescending},
		{MidListView, 'f', mv.openTypeFilter},
		{MidListView, '/', mv.openSearch},
		{MidListView, 's', mv.toggleShowMode},
		{MidListView, 'c', mv.toggleColsMode},
		{MidListView, gocui.KeyEnter, mv.midSelectAsRoot},
		{LeftListView, 'j', mv.sideDown(PaneLeft)},
		{LeftListView, 'k', mv.sideUp(PaneLeft)},
		{LeftListView, gocui.KeyPgdn, mv.sidePageDown(PaneLeft)},
		{LeftListView, gocui.KeyPgup, mv.sidePageUp(PaneLeft)},
		{LeftListView, gocui.KeyEnter, mv.sideSelectAsRoot(PaneLeft)},
		{LeftListView, '/', mv.openSearch},
		{LeftListView, 'f', mv.openTypeFilter},
		{RightListView, 'j', mv.sideDown(PaneRight)},
		{RightListView, 'k', mv.sideUp(PaneRight)},
		{RightListView, gocui.KeyPgdn, mv.sidePageDown(PaneRight)},
		{RightListView, gocui.KeyPgup, mv.sidePageUp(PaneRight)},
		{RightListView, gocui.KeyEnter, mv.sideSelectAsRoot(PaneRight)},
		{RightListView, '/', mv.openSearch},
		{RightListView, 'f', mv.openTypeFilter},
		{TypeFilterView, 'j', mv.filterDown},
		{TypeFilterView, 'k', mv.filterUp},
		{TypeFilterView, gocui.KeyEnter, mv.filterToggle},
	}
	// Space, ?, q, and Q are bound per list view rather than globally: the
	// search box is Editable, and a global binding intercepts a key before
	// it ever reaches the Editor (see gocui's onKey), which would swallow
	// a literal space, "?", "q", or "Q" typed into a search query.
	for _, name := range []string{MidListView, LeftListView, RightListView} {
		bindings = append(bindings,
			keyBinding{name, gocui.KeySpace, mv.cycleFocus},
			keyBinding{name, '?', mv.openHelp},
			keyBinding{name, 'q', mv.goUp},
			keyBinding{name, 'Q', mv.goTop},
		)
	}
	for _, b := range bindings {
		if err := g.SetKeybinding(b.view, b.key, gocui.ModNone, b.fn); err != nil {
			return err
		}
	}
	// ESC is the one truly global key: it always means "back out of
	// whatever's currently active" (help, the type filter, or search),
	// regardless of which view -- including an Editable one -- is current.
	return g.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, mv.closeOverlay)
}

func (mv *MainView) cycleFocus(g *gocui.Gui, v *gocui.View) error {
	switch mv.focus {
	case PaneLeft:
		mv.focus = PaneMid
	case PaneMid:
		mv.focus = PaneRight
	case PaneRight:
		mv.focus = PaneLeft
	}
	return nil
}

func (mv *MainView) openHelp(g *gocui.Gui, v *gocui.View) error {
	mv.mode = ModeHelp
	return nil
}

// openTypeFilter opens the type-filter dialog. Bound to all three list
// views, it operates on whichever one is focused (see activeTypeFilter).
func (mv *MainView) openTypeFilter(g *gocui.Gui, v *gocui.View) error {
	mv.mode = ModeTypeFilter
	return nil
}

// openSearch opens the search box. Bound to all three list views, it
// operates on whichever one is focused (see activeSearchQuery).
func (mv *MainView) openSearch(g *gocui.Gui, v *gocui.View) error {
	mv.mode = ModeSearch
	return nil
}

// closeOverlay backs out of whichever modal is currently active. Search is
// the one case that also reverts state: ESC there means "cancel", so
// whichever query is active (see activeSearchQuery) is cleared; Enter
// (handled in Edit, below) is the "keep it" exit instead. Clearing the mid
// pane's own query cascades to the side panes' too (see
// resetAllSearchQueries) since they're considered part of its search
// scope; clearing a side query only clears that one. With no modal open,
// ESC instead means "go up a root level" -- the same action as lower-case
// q.
func (mv *MainView) closeOverlay(g *gocui.Gui, v *gocui.View) error {
	switch mv.mode {
	case ModeSearch:
		mv.mode = ModeNormal
		if mv.focus == PaneMid {
			mv.resetAllSearchQueries()
			mv.applyFilters()
		} else {
			mv.setActiveSearchQuery("")
		}
	case ModeHelp, ModeTypeFilter:
		mv.mode = ModeNormal
	default:
		mv.goUpRoot()
	}
	return nil
}

func (mv *MainView) goUp(g *gocui.Gui, v *gocui.View) error {
	mv.goUpRoot()
	return nil
}

func (mv *MainView) goTop(g *gocui.Gui, v *gocui.View) error {
	mv.goToTopRoot()
	return nil
}

// toggleShowMode flips between showing root's direct children and its full
// descendant set.
func (mv *MainView) toggleShowMode(g *gocui.Gui, v *gocui.View) error {
	if mv.showMode == ShowChildren {
		mv.showMode = ShowDescendants
	} else {
		mv.showMode = ShowChildren
	}
	mv.updateRootMembers()
	mv.applyFilters()
	return nil
}

// midSelectAsRoot descends into the mid pane's current row.
func (mv *MainView) midSelectAsRoot(g *gocui.Gui, v *gocui.View) error {
	mv.setRoot(mv.current)
	return nil
}

// sideSelectAsRoot descends into whichever item is highlighted in the
// References or Referenced By pane.
func (mv *MainView) sideSelectAsRoot(pane Pane) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		items := mv.sideItems(pane)
		_, cy := v.Cursor()
		_, oy := v.Origin()
		idx := cy + oy
		if idx < 0 || idx >= len(items) {
			return nil
		}
		mv.setRoot(items[idx])
		return nil
	}
}

func (mv *MainView) filterDown(g *gocui.Gui, v *gocui.View) error {
	return moveCursor(v, mv.filterRowCount(), 1)
}

func (mv *MainView) filterUp(g *gocui.Gui, v *gocui.View) error {
	return moveCursor(v, mv.filterRowCount(), -1)
}

// filterToggle applies whichever row of the type filter dialog the cursor
// is on -- the Select All / Deselect All bulk actions, or one kind's
// checkbox -- to whichever of the three type filters is active (see
// activeTypeFilter), then re-applies the appropriate filters/recompute for
// that pane immediately.
func (mv *MainView) filterToggle(g *gocui.Gui, v *gocui.View) error {
	_, cy := v.Cursor()
	_, oy := v.Origin()
	row := cy + oy
	tf := mv.activeTypeFilter()
	switch row {
	case 0:
		for _, kind := range mv.graph.Kinds {
			tf[kind] = true
		}
	case 1:
		for _, kind := range mv.graph.Kinds {
			tf[kind] = false
		}
	default:
		idx := row - 2
		if idx >= 0 && idx < len(mv.graph.Kinds) {
			kind := mv.graph.Kinds[idx]
			tf[kind] = !tf[kind]
		}
	}
	switch mv.focus {
	case PaneLeft, PaneRight:
		// The side panes' own entries (and their RT/RF counts) are rolled
		// up through AccumulatedReferences using this same filter, so
		// they need recomputing outright, not just re-filtering.
		mv.refreshAccumulatedReferences()
		mv.recomputeRefCounts()
		// RefsIn/RefsOut/RefsWithin may have changed for every row, and
		// the mid list might be sorted by one of them, so re-sort along
		// with everything else applyFilters already keeps in sync.
		mv.applyFilters()
	default:
		mv.applyFilters()
	}
	return nil
}

// Edit implements gocui.Editor for the search box: printable characters
// and backspace update whichever query is active (see activeSearchQuery /
// setActiveSearchQuery, dispatched on focus) and re-filter live; Enter
// exits search mode while keeping the query (ESC, handled globally by
// closeOverlay, clears it instead).
func (mv *MainView) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	query := mv.activeSearchQuery()
	switch {
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		if len(query) == 0 {
			return
		}
		query = query[:len(query)-1]
	case key == gocui.KeyEnter:
		mv.mode = ModeNormal
		return
	case key == gocui.KeySpace:
		query += " "
	case ch != 0:
		query += string(ch)
	default:
		return
	}
	mv.setActiveSearchQuery(query)
}

// colIndex returns selectedCol's position among activeColumns, or -1 if
// it's not currently visible (shouldn't normally happen -- see
// toggleColsMode -- but defends against it anyway).
func (mv *MainView) colIndex() int {
	for i, c := range mv.activeColumns() {
		if c == mv.selectedCol {
			return i
		}
	}
	return -1
}

func (mv *MainView) colLeft(g *gocui.Gui, v *gocui.View) error {
	cols := mv.activeColumns()
	idx := mv.colIndex()
	if idx > 0 {
		mv.selectedCol = cols[idx-1]
		// The current row's marker (see formatMidRow) sits at the
		// selected column, so moving the column requires a repaint just
		// like moving the row does (see refreshSelection).
		mv.dirty[MidListView] = true
	}
	return nil
}

func (mv *MainView) colRight(g *gocui.Gui, v *gocui.View) error {
	cols := mv.activeColumns()
	idx := mv.colIndex()
	if idx >= 0 && idx < len(cols)-1 {
		mv.selectedCol = cols[idx+1]
		mv.dirty[MidListView] = true
	}
	return nil
}

// toggleColsMode flips between the Components (LoC/CH/DE/PU/PV/TE) and
// Connectors (RI/RO/RW) column sets. If selectedCol isn't in the new set
// (it was one of the other mode's columns), it falls back to Type.
func (mv *MainView) toggleColsMode(g *gocui.Gui, v *gocui.View) error {
	if mv.colsMode == ColsComponents {
		mv.colsMode = ColsConnectors
	} else {
		mv.colsMode = ColsComponents
	}
	if mv.colIndex() < 0 {
		mv.selectedCol = ColType
	}
	mv.dirty[MidListView] = true
	return nil
}

func (mv *MainView) sortAscending(g *gocui.Gui, v *gocui.View) error {
	return mv.applySort(v, true)
}

func (mv *MainView) sortDescending(g *gocui.Gui, v *gocui.View) error {
	return mv.applySort(v, false)
}

// applySort re-orders allNames by the selected column and, via
// applyFilters, rebuilds the filtered names from it and jumps the mid
// pane's selection to the new top row.
func (mv *MainView) applySort(v *gocui.View, ascending bool) error {
	mv.sortCol = mv.selectedCol
	mv.sortAsc = ascending
	mv.applyFilters()
	return nil
}

// sortComponents sorts names (already filtered down to what will be
// displayed) by col, ascending or descending, with a stable tie-break on
// Name so equal-valued rows don't jump around from one sort to the next.
func sortComponents(graph *db.DB, names []string, col midColumn, ascending bool) {
	sort.Slice(names, func(i, j int) bool {
		a := graph.Components[names[i]]
		b := graph.Components[names[j]]
		cmp := compareComponents(col, a, b)
		if cmp == 0 && col != ColName {
			cmp = strings.Compare(a.Name, b.Name)
		}
		if ascending {
			return cmp < 0
		}
		return cmp > 0
	})
}

func compareComponents(col midColumn, a, b db.Component) int {
	switch col {
	case ColType:
		return strings.Compare(a.Kind, b.Kind)
	case ColName:
		return strings.Compare(a.Name, b.Name)
	case ColLoC:
		return a.LoC - b.LoC
	case ColCH:
		return a.Children - b.Children
	case ColDE:
		return a.Descendants - b.Descendants
	case ColPU:
		return a.PublicCount - b.PublicCount
	case ColPV:
		return a.PrivateCount - b.PrivateCount
	case ColTE:
		return a.TestCount - b.TestCount
	case ColRI:
		return a.RefsIn - b.RefsIn
	case ColRO:
		return a.RefsOut - b.RefsOut
	case ColRW:
		return a.RefsWithin - b.RefsWithin
	case ColLG:
		return a.LevelGap - b.LevelGap
	default:
		return 0
	}
}

func (mv *MainView) midDown(g *gocui.Gui, v *gocui.View) error {
	if err := moveCursor(v, len(mv.names), 1); err != nil {
		return err
	}
	mv.refreshSelection()
	return nil
}

func (mv *MainView) midUp(g *gocui.Gui, v *gocui.View) error {
	if err := moveCursor(v, len(mv.names), -1); err != nil {
		return err
	}
	mv.refreshSelection()
	return nil
}

func (mv *MainView) midPageDown(g *gocui.Gui, v *gocui.View) error {
	if err := pageMove(v, len(mv.names), 1); err != nil {
		return err
	}
	mv.refreshSelection()
	return nil
}

func (mv *MainView) midPageUp(g *gocui.Gui, v *gocui.View) error {
	if err := pageMove(v, len(mv.names), -1); err != nil {
		return err
	}
	mv.refreshSelection()
	return nil
}

func (mv *MainView) sideDown(pane Pane) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return moveCursor(v, len(mv.sideItems(pane)), 1)
	}
}

func (mv *MainView) sideUp(pane Pane) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return moveCursor(v, len(mv.sideItems(pane)), -1)
	}
}

func (mv *MainView) sidePageDown(pane Pane) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return pageMove(v, len(mv.sideItems(pane)), 1)
	}
}

func (mv *MainView) sidePageUp(pane Pane) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		return pageMove(v, len(mv.sideItems(pane)), -1)
	}
}

// sideItems returns what pane's list actually displays -- the
// search-filtered view (see refreshSideFilters), since that's what j/k,
// PgUp/PgDn, and Enter (sideSelectAsRoot) all need to index into.
func (mv *MainView) sideItems(pane Pane) []string {
	if pane == PaneLeft {
		return mv.filteredInbound
	}
	return mv.filteredOutbound
}

// reverseVideo wraps s in the ANSI SGR codes gocui understands (see
// escape.go) to render it in reverse video, independent of the view's own
// foreground/background colors.
func reverseVideo(s string) string {
	return "\x1b[7m" + s + "\x1b[0m"
}

// midColSpec describes one mid-table column's display width and alignment.
type midColSpec struct {
	col   midColumn
	width int
	left  bool
}

// midColumnLayout returns the mid table's columns in display order: Type
// and Name are always present, followed by whichever set colsMode
// currently selects (LoC/CH/DE/PU/PV/TE, or RI/RO/RW). Every column,
// including the first, gets a 2-character marker slot immediately before
// it (see rowMarker / formatMidHeaderLine / formatMidRow) -- shared here
// so the header and every data row line up exactly the same way.
func (mv *MainView) midColumnLayout(nameWidth int) []midColSpec {
	cols := []midColSpec{
		{ColType, widthType, true},
		{ColName, nameWidth, true},
	}
	switch mv.colsMode {
	case ColsComponents:
		cols = append(cols,
			midColSpec{ColLoC, widthLoC, false},
			midColSpec{ColCH, widthCH, false},
			midColSpec{ColDE, widthDE, false},
			midColSpec{ColPU, widthPU, false},
			midColSpec{ColPV, widthPV, false},
			midColSpec{ColTE, widthTE, false},
		)
	case ColsConnectors:
		cols = append(cols,
			midColSpec{ColRI, widthRI, false},
			midColSpec{ColRO, widthRO, false},
			midColSpec{ColRW, widthRW, false},
			midColSpec{ColLG, widthLG, false},
		)
	}
	return cols
}

// midFixedColsWidth is the width consumed by every active column except
// Name, plus the 2-character marker slot ("> " on the selected column of
// the current row, else blank) preceding each of them -- the budget
// midColumnLayout's nameWidth is computed against.
func (mv *MainView) midFixedColsWidth() int {
	layout := mv.midColumnLayout(0)
	total := 2 * len(layout) // every column's marker slot, Name's included
	for _, spec := range layout {
		if spec.col != ColName {
			total += spec.width
		}
	}
	return total
}

// activeColumns is the ordered list of columns colsMode currently shows,
// for h/l navigation (see colLeft/colRight) and validating selectedCol
// after a mode toggle.
func (mv *MainView) activeColumns() []midColumn {
	layout := mv.midColumnLayout(0)
	cols := make([]midColumn, len(layout))
	for i, spec := range layout {
		cols[i] = spec.col
	}
	return cols
}

func padCell(text string, width int, left bool) string {
	if left {
		return fmt.Sprintf("%-*s", width, text)
	}
	return fmt.Sprintf("%*s", width, text)
}

// sortedKeys returns m's keys in sorted order, for stable list display.
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// blankMarker is the 2-character marker slot's default: no row is current,
// or this isn't the selected column, so there's nothing to point at.
const blankMarker = "  "

// rowMarker returns "> " for the selected column's leading slot on the
// current row, else a blank slot of the same width. The header never gets
// a marker -- it shows the selected column via reverse video instead (see
// formatMidHeaderLine).
func (mv *MainView) rowMarker(col midColumn, isCurrent bool) string {
	if isCurrent && col == mv.selectedCol {
		return "> "
	}
	return blankMarker
}

// rootPathText renders the "Root: ..." row: name -> ... -> root, from
// root's highest ancestor down to itself, truncated from the front (whole
// path segments dropped, replaced with a leading "...") if it doesn't fit
// in width.
func (mv *MainView) rootPathText(width int) string {
	const prefix = "Root: "
	if mv.root == "" {
		return prefix + "(none -- showing all)"
	}
	avail := width - len(prefix)
	if avail < 1 {
		avail = 1
	}
	path := mv.graph.AncestorPath(mv.root)
	full := strings.Join(path, " -> ")
	if len(full) <= avail {
		return prefix + full
	}
	for len(path) > 1 {
		path = path[1:]
		candidate := "... -> " + strings.Join(path, " -> ")
		if len(candidate) <= avail {
			return prefix + candidate
		}
	}
	// Even the root's own name plus the marker doesn't fit; fall back to a
	// plain byte truncation of just that name.
	return prefix + "..." + truncate(path[0], avail-3)
}

// searchRowText renders the "Search: ..." row, so the active query stays
// visible even after the search box itself (see layoutSearchBar) is
// closed. See closeOverlay, setRoot, goUpRoot, and goToTopRoot for when
// searchQuery gets reset.
func (mv *MainView) searchRowText() string {
	if mv.searchQuery == "" {
		return "Search: (none)"
	}
	return "Search: " + mv.searchQuery
}

// typesRowText renders a "Types: n of m" row for the given type filter: n
// is how many of the graph's Kinds are currently checked in it, m how many
// Kinds exist in total. Shared by the mid pane and both side panes, each
// with their own filter (typeFilter, leftTypeFilter, rightTypeFilter).
func (mv *MainView) typesRowText(tf map[string]bool) string {
	n := 0
	for _, checked := range tf {
		if checked {
			n++
		}
	}
	return fmt.Sprintf("Types: %d of %d", n, len(mv.graph.Kinds))
}

// modeRowText renders the "Show: ..." row, highlighting whichever of
// Children/Descendants is currently active.
func (mv *MainView) modeRowText() string {
	children, descendants := "Children", "Descendants"
	if mv.showMode == ShowChildren {
		children = reverseVideo(children)
	} else {
		descendants = reverseVideo(descendants)
	}
	return "Show: " + children + "  " + descendants
}

// colsRowText renders the "Cols: ..." row, highlighting whichever of
// Components/Connectors is currently active.
func (mv *MainView) colsRowText() string {
	components, connectors := "Components", "Connectors"
	if mv.colsMode == ColsComponents {
		components = reverseVideo(components)
	} else {
		connectors = reverseVideo(connectors)
	}
	return "Cols: " + components + "  " + connectors
}

// formatMidHeaderLine renders the mid table's column header: the selected
// column's label in reverse video, and a direction arrow appended to
// mv.sortCol's label if names is currently sorted.
func (mv *MainView) formatMidHeaderLine(nameWidth int) string {
	var b strings.Builder
	for _, spec := range mv.midColumnLayout(nameWidth) {
		b.WriteString(blankMarker)
		text := spec.col.label()
		if spec.col == mv.sortCol {
			if mv.sortAsc {
				text += " ▲"
			} else {
				text += " ▼"
			}
		}
		cell := padCell(text, spec.width, spec.left)
		if spec.col == mv.selectedCol {
			cell = reverseVideo(cell)
		}
		b.WriteString(cell)
	}
	return b.String()
}

// formatMidRow renders one data row. If it's the current row (c.Name ==
// mv.current), the selected column gets a leading "> " marker; every other
// slot, on this row or any other, is blank.
func (mv *MainView) formatMidRow(nameWidth int, c db.Component, isCurrent bool) string {
	var b strings.Builder
	for _, spec := range mv.midColumnLayout(nameWidth) {
		b.WriteString(mv.rowMarker(spec.col, isCurrent))
		b.WriteString(padCell(midColumnValue(spec.col, c, nameWidth), spec.width, spec.left))
	}
	return b.String()
}

func midColumnValue(col midColumn, c db.Component, nameWidth int) string {
	switch col {
	case ColType:
		return truncate(c.Kind, widthType)
	case ColName:
		return truncate(c.Name, nameWidth)
	case ColLoC:
		return fmt.Sprintf("%d", c.LoC)
	case ColCH:
		return fmt.Sprintf("%d", c.Children)
	case ColDE:
		return fmt.Sprintf("%d", c.Descendants)
	case ColPU:
		return fmt.Sprintf("%d", c.PublicCount)
	case ColPV:
		return fmt.Sprintf("%d", c.PrivateCount)
	case ColTE:
		return fmt.Sprintf("%d", c.TestCount)
	case ColRI:
		return fmt.Sprintf("%d", c.RefsIn)
	case ColRO:
		return fmt.Sprintf("%d", c.RefsOut)
	case ColRW:
		return fmt.Sprintf("%d", c.RefsWithin)
	case ColLG:
		return fmt.Sprintf("%d", c.LevelGap)
	default:
		return ""
	}
}

// truncate shortens s to at most width bytes, marking that it was cut with
// a trailing ellipsis so table columns stay aligned regardless of value
// length. Component names and kinds in this domain are ASCII identifiers,
// so a byte-based cut is sufficient.
func truncate(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width <= 1 {
		return s[:width]
	}
	return s[:width-1] + "…"
}

// moveCursor advances a list view's cursor by delta rows, scrolling the
// origin if needed, and refusing to move past either end of length.
func moveCursor(v *gocui.View, length, delta int) error {
	if length == 0 {
		return nil
	}
	cx, cy := v.Cursor()
	ox, oy := v.Origin()
	idx := cy + oy + delta
	if idx < 0 || idx >= length {
		return nil
	}
	if delta > 0 {
		if err := v.SetCursor(cx, cy+1); err != nil {
			return v.SetOrigin(ox, oy+1)
		}
		return nil
	}
	if err := v.SetCursor(cx, cy-1); err != nil {
		if oy > 0 {
			return v.SetOrigin(ox, oy-1)
		}
	}
	return nil
}

// pageMove jumps a list view's cursor by one visible page (its own
// height), clamped to [0, length), landing roughly centered in the new
// viewport. pages should be +1 (PgDn) or -1 (PgUp).
func pageMove(v *gocui.View, length, pages int) error {
	if length == 0 {
		return nil
	}
	_, height := v.Size()
	if height < 1 {
		height = 1
	}
	cx, cy := v.Cursor()
	ox, oy := v.Origin()
	target := cy + oy + pages*height
	if target < 0 {
		target = 0
	}
	if target >= length {
		target = length - 1
	}
	maxOrigin := length - height
	if maxOrigin < 0 {
		maxOrigin = 0
	}
	newOrigin := target - height/2
	if newOrigin < 0 {
		newOrigin = 0
	}
	if newOrigin > maxOrigin {
		newOrigin = maxOrigin
	}
	if err := v.SetOrigin(ox, newOrigin); err != nil {
		return err
	}
	return v.SetCursor(cx, target-newOrigin)
}
