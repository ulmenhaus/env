// Package db loads a jql codedb graph (as produced by jql-codedb-extract)
// and indexes it for fast, interactive traversal.
package db

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
)

// Component describes a single node or subsystem within a codedb graph,
// along with metrics that are cheap to look up here only because they were
// computed once, at load time, over the whole graph (see Load).
type Component struct {
	Name string
	// LowerName is Name lower-cased, precomputed once here so a live,
	// case-insensitive search filter can do a plain substring check per
	// keystroke instead of lower-casing every name on every keystroke.
	LowerName   string
	Kind        string
	SrcLocation string
	SoParent    string

	// LoC is cumulative: this component's own lines of code plus the LoC
	// of every component recursively contained under it (see Descendants).
	LoC int

	Children    int // components whose SoParent is this component
	Descendants int // components recursively contained under this one

	// PublicCount, PrivateCount, and TestCount are how many components in
	// this component's own subtree (itself plus every descendant) fall
	// into each of the three mutually exclusive categories categorize
	// assigns: a component with Test == "yes" is a test component;
	// otherwise it's public if Top == "yes", private otherwise. Rolled up
	// in the same post-order pass as Descendants/LoC, so this is a single
	// map lookup regardless of graph size.
	PublicCount  int
	PrivateCount int
	TestCount    int

	// RefsIn, RefsOut, and RefsWithin are all accumulated through this
	// component's whole containment subtree (see
	// computeAccumulatedRefCounts): a reference from a to b counts toward
	// every ancestor-or-self of a's RefsOut and every ancestor-or-self of
	// b's RefsIn, EXCEPT where some ancestor-or-self is common to both --
	// there it counts toward that component's RefsWithin instead, not
	// RefsIn or RefsOut (it doesn't cross that component's boundary).
	// Precomputed once for every component at load time so the UI can
	// sort/display these for the whole table at once.
	RefsIn     int
	RefsOut    int
	RefsWithin int

	// RootLevel is the level of this component's top-level ancestor (its
	// "package" -- see computePackageLevels): a top-level component
	// (empty SoParent) that references no other top-level component is
	// level 1; one that only references level-1 components is level 2;
	// and so on, one more than the longest chain of top-level references
	// starting from it. Every component under a given top-level ancestor
	// shares that ancestor's level.
	RootLevel int

	// LevelGap is the spread of RootLevel among the components this
	// one's subtree references externally -- the same set RefsOut counts
	// (see computeAccumulatedRefCounts) -- computed as the highest such
	// RootLevel minus the lowest, or 0 if RefsOut is 0.
	LevelGap int
}

type rawComponent struct {
	Kind        string `json:"Kind"`
	LoC         int    `json:"LoC"`
	SrcLocation string `json:"SrcLocation"`
	SoParent    string `json:"SoParent"`
	Test        string `json:"Test"`
	Top         string `json:"Top"`
}

// category is one of the three mutually exclusive buckets every component
// falls into, used to roll up PublicCount/PrivateCount/TestCount.
type category int

const (
	categoryPublic category = iota
	categoryPrivate
	categoryTest
)

// categorize implements the rule: Test == "yes" makes a component a test
// component regardless of Top; otherwise it's public if Top == "yes", else
// private.
func categorize(rc rawComponent) category {
	if rc.Test == "yes" {
		return categoryTest
	}
	if rc.Top == "yes" {
		return categoryPublic
	}
	return categoryPrivate
}

type rawReference struct {
	SDSource string `json:"SDSource"`
	SDDest   string `json:"SDDest"`
}

type rawDatabase struct {
	Components     map[string]rawComponent `json:"components"`
	BaseReferences map[string]rawReference `json:"base_references"`
}

// DB is an in-memory, indexed view of a codedb graph. It is built once at
// load time so that lookups used during interactive navigation -- what does
// this component reference, what references it -- are O(1) map accesses
// rather than scans, which matters once base_references grows into the
// millions of rows.
type DB struct {
	Components map[string]Component
	// Names holds every component name, sorted, for stable iteration order.
	Names []string
	// Kinds holds every distinct Component.Kind, sorted, for populating the
	// type-filter dialog. KindCounts is how many components have each kind.
	Kinds      []string
	KindCounts map[string]int

	// outCounts[source][dest] and inCounts[dest][source] both hold how many
	// individual base_references point from source directly to dest --
	// same data, indexed the other way round so a component's direct
	// outbound or inbound edges are each a single map lookup regardless of
	// graph size. AccumulatedReferences walks these to add up a whole
	// subtree's edges on demand.
	outCounts map[string]map[string]int
	inCounts  map[string]map[string]int

	// children[parent] holds every component whose SoParent is parent.
	// Built once at load time; AccumulatedReferences walks it lazily, at
	// display time, rather than this package precomputing a rolled-up
	// reference count for every component up front.
	children map[string][]string

	// rootLevel maps every component to its top-level ancestor's level
	// (see Component.RootLevel and computePackageLevels). Kept here,
	// unexported, purely so RecomputeAccumulatedRefCounts can pass it
	// straight back into computeAccumulatedRefCounts when a type-filter
	// change forces LevelGap to be recomputed alongside
	// RefsIn/RefsOut/RefsWithin -- levels themselves never change after
	// Load, only which edges count toward LevelGap does.
	rootLevel map[string]int
}

// Load reads and indexes the codedb graph at path.
func Load(path string) (*DB, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw rawDatabase
	if err := json.Unmarshal(contents, &raw); err != nil {
		return nil, err
	}

	d := &DB{
		Components: make(map[string]Component, len(raw.Components)),
		Names:      make([]string, 0, len(raw.Components)),
	}
	kindCounts := make(map[string]int)
	for name, rc := range raw.Components {
		d.Components[name] = Component{
			Name:        name,
			LowerName:   strings.ToLower(name),
			Kind:        rc.Kind,
			SrcLocation: rc.SrcLocation,
			SoParent:    rc.SoParent,
			LoC:         rc.LoC,
		}
		d.Names = append(d.Names, name)
		kindCounts[rc.Kind]++
	}
	sort.Strings(d.Names)

	d.KindCounts = kindCounts
	d.Kinds = make([]string, 0, len(kindCounts))
	for kind := range kindCounts {
		d.Kinds = append(d.Kinds, kind)
	}
	sort.Strings(d.Kinds)

	// Tally per-pair edge counts in one pass; these serve the side panes'
	// per-selection reference lists (see AccumulatedReferences).
	outCounts := make(map[string]map[string]int, len(raw.Components))
	inCounts := make(map[string]map[string]int, len(raw.Components))
	for _, ref := range raw.BaseReferences {
		if ref.SDSource == "" || ref.SDDest == "" {
			continue
		}
		if outCounts[ref.SDSource] == nil {
			outCounts[ref.SDSource] = map[string]int{}
		}
		outCounts[ref.SDSource][ref.SDDest]++

		if inCounts[ref.SDDest] == nil {
			inCounts[ref.SDDest] = map[string]int{}
		}
		inCounts[ref.SDDest][ref.SDSource]++
	}
	d.outCounts = outCounts
	d.inCounts = inCounts

	// children[parent] holds every component whose SoParent is parent, used
	// below to compute each component's direct child count and, via a
	// single post-order pass, its full recursive descendant count -- and
	// later, by AccumulatedReferences and DirectChildren/DescendantNames,
	// to walk a subtree on demand. A component with no SoParent is filed
	// under children[""], the sentinel for "the forest's own root," so
	// DirectChildren("") uniformly gives the top-level components.
	children := make(map[string][]string, len(raw.Components))
	for name, rc := range raw.Components {
		children[rc.SoParent] = append(children[rc.SoParent], name)
	}
	d.children = children
	rawLoC := make(map[string]int, len(raw.Components))
	categories := make(map[string]category, len(raw.Components))
	for name, rc := range raw.Components {
		rawLoC[name] = rc.LoC
		categories[name] = categorize(rc)
	}
	totals := computeSubtreeMetrics(d.Names, children, rawLoC, categories)

	// rootOf maps every component to its top-level ancestor -- its
	// "package" for RootLevel/LevelGap purposes, regardless of that
	// ancestor's actual Kind. AncestorPath's chain is only a few levels
	// deep in practice (see its own doc comment's reasoning, reused
	// throughout this file), so doing this once per name is still an
	// O(N) pass overall.
	rootOf := make(map[string]string, len(d.Names))
	for _, name := range d.Names {
		path := d.AncestorPath(name)
		if len(path) == 0 {
			rootOf[name] = name
		} else {
			rootOf[name] = path[0]
		}
	}

	// topGraph[a][b] records that some component under top-level
	// package a directly references some component under top-level
	// package b (a != b) -- built from the same outCounts tally as
	// everything else, in one O(edges) pass, purely to feed
	// computePackageLevels below.
	topGraph := make(map[string]map[string]bool)
	for source, dests := range d.outCounts {
		rs := rootOf[source]
		for dest := range dests {
			rd := rootOf[dest]
			if rs == rd {
				continue
			}
			if topGraph[rs] == nil {
				topGraph[rs] = map[string]bool{}
			}
			topGraph[rs][rd] = true
		}
	}
	packageLevel := computePackageLevels(children[""], topGraph)
	rootLevel := make(map[string]int, len(d.Names))
	for _, name := range d.Names {
		rootLevel[name] = packageLevel[rootOf[name]]
	}
	d.rootLevel = rootLevel

	accumIn, accumOut, accumWithin, accumLevelGap := computeAccumulatedRefCounts(d, d.outCounts, nil, nil, rootLevel)

	for name, c := range d.Components {
		t := totals[name]
		c.Children = len(children[name])
		c.Descendants = t.descendants
		c.LoC = t.loc
		c.PublicCount = t.public
		c.PrivateCount = t.private
		c.TestCount = t.test
		c.RefsIn = accumIn[name]
		c.RefsOut = accumOut[name]
		c.RefsWithin = accumWithin[name]
		c.RootLevel = rootLevel[name]
		c.LevelGap = accumLevelGap[name]
		d.Components[name] = c
	}

	return d, nil
}

// computePackageLevels assigns every top-level component ("package," in
// this file's shorthand -- any component with an empty SoParent) a
// level: a package that references no other package (topGraph[name] is
// empty) is level 1; a package whose only package references are to
// level-1 packages is level 2; and so on -- one more than the longest
// chain of package-to-package references starting from it. Memoized
// (each package visited once regardless of how many others reference
// it), so this is O(packages + package-to-package edges), not O(edges)
// over the whole graph.
func computePackageLevels(topLevelNames []string, topGraph map[string]map[string]bool) map[string]int {
	levels := make(map[string]int, len(topLevelNames))
	const (
		unvisited = iota
		inProgress
		done
	)
	state := make(map[string]int, len(topLevelNames))
	var visit func(name string) int
	visit = func(name string) int {
		switch state[name] {
		case done:
			return levels[name]
		case inProgress:
			// A genuine cycle between packages shouldn't arise from a
			// real import graph (Go itself rejects import cycles), but
			// if the extracted edges do form one, this back edge is
			// treated as contributing nothing further rather than
			// recursing forever.
			return 0
		}
		state[name] = inProgress
		maxDepLevel := 0
		for dep := range topGraph[name] {
			if l := visit(dep); l > maxDepLevel {
				maxDepLevel = l
			}
		}
		levels[name] = maxDepLevel + 1
		state[name] = done
		return levels[name]
	}
	for _, name := range topLevelNames {
		if state[name] == unvisited {
			visit(name)
		}
	}
	return levels
}

// subtreeTotals holds one component's rolled-up totals over its own
// subtree (itself plus every descendant): how many components that is
// (descendants alone; +1 for the component itself gives the subtree
// size), their combined LoC, and how many fall into each of the three
// categorize buckets.
type subtreeTotals struct {
	descendants int
	loc         int
	public      int
	private     int
	test        int
}

// computeSubtreeMetrics walks the children (SoParent) relation once, in
// post-order, to compute every name's subtreeTotals in a single pass.
// SoParent is expected to form a forest (each component has at most one
// parent), so this is a single O(V) pass; a visited guard keeps it safe
// (if not fully accurate for the affected nodes) even if malformed input
// introduces a cycle.
func computeSubtreeMetrics(names []string, children map[string][]string, loc map[string]int, categories map[string]category) map[string]subtreeTotals {
	const (
		unvisited = iota
		inProgress
		done
	)
	state := make(map[string]int, len(names))
	totals := make(map[string]subtreeTotals, len(names))

	var visit func(name string) subtreeTotals
	visit = func(name string) subtreeTotals {
		switch state[name] {
		case done:
			return totals[name]
		case inProgress:
			// Cycle detected; don't double-count or recurse forever.
			return subtreeTotals{}
		}
		state[name] = inProgress
		t := subtreeTotals{loc: loc[name]}
		switch categories[name] {
		case categoryPublic:
			t.public = 1
		case categoryPrivate:
			t.private = 1
		case categoryTest:
			t.test = 1
		}
		for _, child := range children[name] {
			c := visit(child)
			t.descendants += 1 + c.descendants
			t.loc += c.loc
			t.public += c.public
			t.private += c.private
			t.test += c.test
		}
		totals[name] = t
		state[name] = done
		return t
	}

	for _, name := range names {
		if state[name] == unvisited {
			visit(name)
		}
	}
	return totals
}

// computeAccumulatedRefCounts rolls base_references up through the
// containment hierarchy to give every component precomputed RefsIn,
// RefsOut, and RefsWithin totals (see Component). For a reference from a
// to b: every ancestor-or-self of a gets +1 RefsOut and every
// ancestor-or-self of b gets +1 RefsIn -- UNLESS some ancestor-or-self is
// common to both, in which case the reference is internal to that
// component's own subtree, so it counts toward that component's
// RefsWithin instead, excluded from RefsIn/RefsOut entirely (it never
// crosses that component's boundary).
//
// Ancestor chains are memoized per component (containment hierarchies are
// only a few levels deep in practice, so each chain is tiny), making this
// a single pass over the references -- proportional to edge count, not to
// edges times graph size -- computed once here at load time so the UI can
// display and sort by these for every component at once.
// computeAccumulatedRefCounts iterates outCounts (source -> dest -> edge
// count -- see DB.outCounts) rather than the raw reference list: it's the
// same data, already aggregated by pair, and re-walking it is exactly what
// RecomputeAccumulatedRefCounts needs to redo this on an explicit type-
// filter change without keeping the original per-edge list around.
//
// allowedSourceKinds/allowedDestKinds, if non-nil, restrict which edges
// count at all: an edge is skipped unless some ancestor-or-self of its
// source has an allowed Kind (or allowedSourceKinds is nil, meaning no
// restriction) and likewise for some ancestor-or-self of its dest (see
// firstMatchingAncestor) -- not just the source/dest itself, so an edge to
// a filtered-out leaf still counts as long as one of its containers
// isn't. A skipped edge counts toward neither RefsIn, RefsOut, nor
// RefsWithin -- it's as if it doesn't exist for this computation, which is
// what keeps these in sync with the type-filtered References/Referenced
// By panes (see MainView.recomputeRefCounts in the ui package, and
// AccumulatedReferences below, which applies the same ancestor-aware rule
// to decide what those panes display).
//
// rootLevel (see Component.RootLevel) is looked up for each dest counted
// toward out[x], in the same pass, to also compute levelGap[x] -- the
// highest such RootLevel minus the lowest, i.e. Component.LevelGap for
// every component at once. A dest excluded by allowedDestKinds (or
// classified as within rather than out) never contributes to it, so
// levelGap stays in sync with out/RefsOut exactly the way within/RefsWithin
// already does.
func computeAccumulatedRefCounts(d *DB, outCounts map[string]map[string]int, allowedSourceKinds, allowedDestKinds map[string]bool, rootLevel map[string]int) (in, out, within, levelGap map[string]int) {
	components := d.Components
	in = map[string]int{}
	out = map[string]int{}
	within = map[string]int{}
	gapMin := map[string]int{}
	gapMax := map[string]int{}
	hasGap := map[string]bool{}

	const (
		unvisited = iota
		inProgress
		done
	)
	chains := map[string][]string{}
	chainState := map[string]int{}
	var chainOf func(name string) []string
	chainOf = func(name string) []string {
		if name == "" {
			return nil
		}
		switch chainState[name] {
		case done:
			return chains[name]
		case inProgress:
			return nil // cycle guard
		}
		chainState[name] = inProgress
		parentChain := chainOf(components[name].SoParent)
		chain := make([]string, 0, len(parentChain)+1)
		chain = append(chain, name)
		chain = append(chain, parentChain...)
		chains[name] = chain
		chainState[name] = done
		return chain
	}

	containsName := func(chain []string, name string) bool {
		for _, c := range chain {
			if c == name {
				return true
			}
		}
		return false
	}

	for source, dests := range outCounts {
		if _, ok := d.firstMatchingAncestor(source, allowedSourceKinds); !ok {
			continue
		}
		for dest, count := range dests {
			if _, ok := d.firstMatchingAncestor(dest, allowedDestKinds); !ok {
				continue
			}
			srcChain := chainOf(source)
			dstChain := chainOf(dest)
			for _, x := range srcChain {
				if containsName(dstChain, x) {
					within[x] += count
				} else {
					out[x] += count
					lv := rootLevel[dest]
					if !hasGap[x] {
						gapMin[x], gapMax[x], hasGap[x] = lv, lv, true
					} else if lv < gapMin[x] {
						gapMin[x] = lv
					} else if lv > gapMax[x] {
						gapMax[x] = lv
					}
				}
			}
			for _, y := range dstChain {
				if containsName(srcChain, y) {
					// Already tallied as within[y] in the srcChain loop
					// above; counting it again from this side too would
					// double it, so it's deliberately skipped here (see
					// the doc comment above).
					continue
				}
				in[y] += count
			}
		}
	}
	levelGap = make(map[string]int, len(hasGap))
	for x := range hasGap {
		levelGap[x] = gapMax[x] - gapMin[x]
	}
	return in, out, within, levelGap
}

// RecomputeAccumulatedRefCounts redoes every component's RefsIn, RefsOut,
// and RefsWithin (see Component and computeAccumulatedRefCounts),
// restricted to edges whose source/dest Kind is allowed by
// allowedSourceKinds/allowedDestKinds (nil for either means no
// restriction on that side), and mutates d.Components in place. It's the
// same O(edges) cost as the original load-time computation, so it's meant
// to be called on an explicit, infrequent filter change (see the ui
// package's type-filter dialog), not on every keystroke.
func (d *DB) RecomputeAccumulatedRefCounts(allowedSourceKinds, allowedDestKinds map[string]bool) {
	in, out, within, levelGap := computeAccumulatedRefCounts(d, d.outCounts, allowedSourceKinds, allowedDestKinds, d.rootLevel)
	for name, c := range d.Components {
		c.RefsIn = in[name]
		c.RefsOut = out[name]
		c.RefsWithin = within[name]
		c.LevelGap = levelGap[name]
		d.Components[name] = c
	}
}

// firstMatchingAncestor walks name and then its ancestors (via SoParent)
// and returns the first -- closest to name -- whose Kind is checked in
// allowedKinds, along with true. If none are, including the top-level
// ancestor, it returns ("", false). allowedKinds == nil means no
// restriction: name itself always matches, with no walk needed.
//
// This is what lets a component whose own Kind has been filtered out
// still show up "as" whichever ancestor hasn't been -- see
// AccumulatedReferences and computeAccumulatedRefCounts, which both use
// it to decide what a reference rolls up to under a type filter.
func (d *DB) firstMatchingAncestor(name string, allowedKinds map[string]bool) (string, bool) {
	if allowedKinds == nil {
		return name, true
	}
	visited := map[string]bool{}
	for cur := name; cur != "" && !visited[cur]; cur = d.Components[cur].SoParent {
		visited[cur] = true
		if allowedKinds[d.Components[cur].Kind] {
			return cur, true
		}
	}
	return "", false
}

// AccumulatedReferences returns name's references "rolled up" through its
// containment subtree: if some descendant-or-self of name (including name
// itself) has a direct reference to/from some other component X, that
// counts toward name's totals too. X itself is never a member of name's
// own subtree -- a reference where both ends are under name is internal
// to it (see Component.RefsWithin), not a reference to or from something
// else, so it never appears in out/in.
//
// X is further rolled up to its first ancestor-or-self whose Kind is
// allowed by destFilter (for out) or sourceFilter (for in) -- see
// firstMatchingAncestor -- rather than being excluded outright when X's
// own Kind isn't allowed. Concretely: if component a (under name) and b
// (under some other component B) are related this way, and a references
// b, then when name is selected, B shows up in out even if b itself has
// been filtered out, as long as B (or some ancestor of b closer than B)
// passes destFilter. out[X]/in[X] sum every edge that rolls up to that
// same X, so if two different descendants each reference something under
// X, X shows up once with the combined count rather than as separate
// entries.
//
// This walks name's subtree fresh on every call -- O(size of that
// subtree) -- rather than precomputing a rolled-up count for every
// component at load time. A leaf component's subtree is just itself, so
// viewing one stays cheap even in a graph with hundreds of thousands of
// components; only selecting a large top-level container pays a
// proportionally larger cost, and only when it's actually selected.
func (d *DB) AccumulatedReferences(name string, sourceFilter, destFilter map[string]bool) (out, in map[string]int) {
	// First collect the whole subtree so out/in can tell whether a given
	// target/source is itself part of it -- can't filter while walking,
	// since membership isn't fully known until the walk finishes.
	members := map[string]bool{}
	var collect func(member string)
	collect = func(member string) {
		if members[member] {
			return
		}
		members[member] = true
		for _, child := range d.children[member] {
			collect(child)
		}
	}
	collect(name)

	out = map[string]int{}
	in = map[string]int{}
	for member := range members {
		for dest, count := range d.outCounts[member] {
			if members[dest] {
				continue
			}
			if match, ok := d.firstMatchingAncestor(dest, destFilter); ok {
				out[match] += count
			}
		}
		for source, count := range d.inCounts[member] {
			if members[source] {
				continue
			}
			if match, ok := d.firstMatchingAncestor(source, sourceFilter); ok {
				in[match] += count
			}
		}
	}

	return out, in
}

// DirectChildren returns name's immediate children, sorted. name == ""
// means the forest's own root, so this returns the top-level components
// (those with no SoParent). O(number of children), not graph size.
func (d *DB) DirectChildren(name string) []string {
	kids := append([]string(nil), d.children[name]...)
	sort.Strings(kids)
	return kids
}

// DescendantNames returns every component recursively contained under
// name (not including name itself), sorted. name == "" returns every
// component in the database, since the forest as a whole has no single
// container to descend from. O(size of name's subtree), computed fresh on
// each call rather than precomputed for every component at load time --
// see AccumulatedReferences for the same tradeoff.
func (d *DB) DescendantNames(name string) []string {
	if name == "" {
		return d.Names
	}
	var result []string
	var walk func(member string)
	walk = func(member string) {
		for _, child := range d.children[member] {
			result = append(result, child)
			walk(child)
		}
	}
	walk(name)
	sort.Strings(result)
	return result
}

// AncestorPath returns the chain from name's highest ancestor down to name
// itself (name last), following SoParent. A cycle guard makes it safe (if
// not fully accurate) even if malformed input introduces one.
func (d *DB) AncestorPath(name string) []string {
	var path []string
	visited := map[string]bool{}
	for cur := name; cur != "" && !visited[cur]; cur = d.Components[cur].SoParent {
		visited[cur] = true
		path = append(path, cur)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}
