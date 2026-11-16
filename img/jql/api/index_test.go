package api

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/ulmenhaus/env/img/jql/types"
	"github.com/ulmenhaus/env/proto/jql/jqlpb"
)

const refPattern = `@\{[^}]*\}`

// newAssertionsTable returns an assertions-like table with the given number of rows. If
// indexed is true Arg0 has an exact index and Arg1 has a pattern index for foreign refs.
func newAssertionsTable(t testing.TB, rows int, indexed bool) *types.Table {
	columns := []string{"A Relation", "Arg0", "Arg1", "_Description"}
	constructors := map[string]types.FieldValueConstructor{}
	meta := map[string]*types.ColumnMeta{}
	features := map[string]map[string]interface{}{}
	for _, col := range columns {
		constructors[col] = types.NewString
		meta[col] = &types.ColumnMeta{}
		features[col] = map[string]interface{}{}
	}
	if indexed {
		exact, err := types.ParseIndexSpec(map[string]interface{}{"type": "exact"})
		if err != nil {
			t.Fatal(err)
		}
		pattern, err := types.ParseIndexSpec(map[string]interface{}{"type": "pattern", "pattern": refPattern})
		if err != nil {
			t.Fatal(err)
		}
		meta["Arg0"].Index = exact
		meta["Arg1"].Index = pattern
	}
	table := types.NewTable(columns, map[string][]types.Entry{}, "_Description", constructors, features, meta)
	r := rand.New(rand.NewSource(1))
	for i := 0; i < rows; i++ {
		pk := fmt.Sprintf("assn %d", i)
		if err := table.InsertWithFields(pk, map[string]string{
			"A Relation": fmt.Sprintf(".Rel%d", r.Intn(10)),
			"Arg0":       fmt.Sprintf("nouns n%d", r.Intn(rows/10+1)),
			"Arg1":       randomArg1(r, rows),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return table
}

func randomArg1(r *rand.Rand, rows int) string {
	n := r.Intn(rows/10 + 1)
	switch r.Intn(5) {
	case 0:
		return fmt.Sprintf("@{nouns n%d}", n)
	case 1:
		return fmt.Sprintf("see @{nouns n%d} and @{Nouns N%d}", n, r.Intn(rows/10+1))
	case 2:
		// nested/unterminated refs exercise overlapping matches
		return fmt.Sprintf("@{broken @{nouns n%d}", n)
	case 3:
		return fmt.Sprintf("plain text n%d", n)
	default:
		return ""
	}
}

func queryPKs(t testing.TB, table *types.Table, filters ...*jqlpb.Filter) []string {
	var shims []types.Filter
	for _, f := range filters {
		shims = append(shims, newFilterShim(f, table))
	}
	resp, err := table.Query(types.QueryParams{Filters: shims, OrderBy: "_Description"})
	if err != nil {
		t.Fatal(err)
	}
	var pks []string
	for _, row := range resp.Entries {
		pks = append(pks, row[table.Primary()].Format(""))
	}
	sort.Strings(pks)
	return pks
}

func testFilters() [][]*jqlpb.Filter {
	eq := func(col, v string) *jqlpb.Filter {
		return &jqlpb.Filter{Column: col, Match: &jqlpb.Filter_EqualMatch{EqualMatch: &jqlpb.EqualMatch{Value: v}}}
	}
	in := func(col string, vs ...string) *jqlpb.Filter {
		return &jqlpb.Filter{Column: col, Match: &jqlpb.Filter_InMatch{InMatch: &jqlpb.InMatch{Values: vs}}}
	}
	contains := func(col, v string) *jqlpb.Filter {
		return &jqlpb.Filter{Column: col, Match: &jqlpb.Filter_ContainsMatch{ContainsMatch: &jqlpb.ContainsMatch{Value: v}}}
	}
	neg := func(f *jqlpb.Filter) *jqlpb.Filter {
		f.Negated = true
		return f
	}
	return [][]*jqlpb.Filter{
		{eq("Arg0", "nouns n3")},
		{eq("Arg0", "nouns n3"), eq("A Relation", ".Rel2")},
		{in("Arg0", "nouns n1", "nouns n2", "nouns missing")},
		{in("Arg0")},
		{neg(eq("Arg0", "nouns n3"))},
		{contains("Arg1", "@{nouns n4}")},
		{contains("Arg1", "@{NOUNS N4}")},
		{contains("Arg1", "@{nouns n4}"), in("Arg0", "nouns n1", "nouns n5")},
		{contains("Arg1", "nouns n4")}, // not a full ref so must fall back to a scan
		{eq("Arg1", "@{nouns n7}")},
		{in("Arg1", "@{nouns n7}", "@{nouns n8}")},
		{in("Arg1", "@{nouns n7}", "plain text n7")}, // mixed values fall back to a scan
		{contains("Arg1", "@{broken @{nouns n2}")},
	}
}

func assertSameResults(t *testing.T, indexed, plain *types.Table, step string) {
	for _, filters := range testFilters() {
		want := queryPKs(t, plain, filters...)
		got := queryPKs(t, indexed, filters...)
		if fmt.Sprint(want) != fmt.Sprint(got) {
			var descs []string
			for _, f := range filters {
				descs = append(descs, Description(f))
			}
			t.Fatalf("%s: results differ for %v:\nwant %v\ngot  %v", step, descs, want, got)
		}
	}
}

func TestIndexedQueriesMatchScans(t *testing.T) {
	indexed := newAssertionsTable(t, 500, true)
	plain := newAssertionsTable(t, 500, false)
	assertSameResults(t, indexed, plain, "initial")

	r := rand.New(rand.NewSource(2))
	for i := 0; i < 300; i++ {
		pk := fmt.Sprintf("assn %d", r.Intn(520))
		var step string
		var errs [2]error
		for j, table := range []*types.Table{indexed, plain} {
			rr := rand.New(rand.NewSource(int64(i))) // identical mutations to both tables
			switch rr.Intn(5) {
			case 0:
				step = "update Arg0"
				errs[j] = table.Update(pk, "Arg0", fmt.Sprintf("nouns n%d", rr.Intn(51)))
			case 1:
				step = "update Arg1"
				errs[j] = table.Update(pk, "Arg1", randomArg1(rr, 500))
			case 2:
				step = "rename pk"
				errs[j] = table.Update(pk, "_Description", fmt.Sprintf("assn %d", rr.Intn(520)))
			case 3:
				step = "delete"
				errs[j] = table.Delete(pk)
			case 4:
				step = "insert"
				errs[j] = table.InsertWithFields(pk, map[string]string{
					"Arg0": fmt.Sprintf("nouns n%d", rr.Intn(51)),
					"Arg1": randomArg1(rr, 500),
				})
			}
		}
		if (errs[0] == nil) != (errs[1] == nil) {
			t.Fatalf("%s %s: mismatched errors %v vs %v", step, pk, errs[0], errs[1])
		}
		assertSameResults(t, indexed, plain, fmt.Sprintf("after %s %s", step, pk))
	}
}

func TestParseIndexSpecErrors(t *testing.T) {
	for _, raw := range []interface{}{
		"exact",
		map[string]interface{}{},
		map[string]interface{}{"type": "btree"},
		map[string]interface{}{"type": "pattern"},
		map[string]interface{}{"type": "pattern", "pattern": "("},
	} {
		if _, err := types.ParseIndexSpec(raw); err == nil {
			t.Errorf("expected error for %#v", raw)
		}
	}
}

func benchmarkQuery(b *testing.B, indexed bool, filter *jqlpb.Filter) {
	table := newAssertionsTable(b, 100000, indexed)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		queryPKs(b, table, filter)
	}
}

var (
	benchArg0 = &jqlpb.Filter{Column: "Arg0", Match: &jqlpb.Filter_EqualMatch{EqualMatch: &jqlpb.EqualMatch{Value: "nouns n42"}}}
	benchArg1 = &jqlpb.Filter{Column: "Arg1", Match: &jqlpb.Filter_ContainsMatch{ContainsMatch: &jqlpb.ContainsMatch{Value: "@{nouns n42}"}}}
)

func BenchmarkArg0EqualScan(b *testing.B)       { benchmarkQuery(b, false, benchArg0) }
func BenchmarkArg0EqualIndexed(b *testing.B)    { benchmarkQuery(b, true, benchArg0) }
func BenchmarkArg1ContainsScan(b *testing.B)    { benchmarkQuery(b, false, benchArg1) }
func BenchmarkArg1ContainsIndexed(b *testing.B) { benchmarkQuery(b, true, benchArg1) }

// newTreeTable returns a nouns-like table of rows "n<i>" whose Parent forms a random forest
func newTreeTable(t testing.TB, rows int, indexed bool) *types.Table {
	columns := []string{"Parent", "_Identifier"}
	constructors := map[string]types.FieldValueConstructor{}
	meta := map[string]*types.ColumnMeta{}
	features := map[string]map[string]interface{}{}
	for _, col := range columns {
		constructors[col] = types.NewString
		meta[col] = &types.ColumnMeta{}
		features[col] = map[string]interface{}{}
	}
	if indexed {
		exact, err := types.ParseIndexSpec(map[string]interface{}{"type": "exact"})
		if err != nil {
			t.Fatal(err)
		}
		meta["Parent"].Index = exact
	}
	table := types.NewTable(columns, map[string][]types.Entry{}, "_Identifier", constructors, features, meta)
	r := rand.New(rand.NewSource(3))
	for i := 0; i < rows; i++ {
		parent := ""
		if i > 0 && r.Intn(10) != 0 {
			parent = fmt.Sprintf("n%d", r.Intn(i))
		}
		if err := table.InsertWithFields(fmt.Sprintf("n%d", i), map[string]string{"Parent": parent}); err != nil {
			t.Fatal(err)
		}
	}
	return table
}

func TestPathAndPrimaryQueriesMatchScans(t *testing.T) {
	path := func(value string, reverse bool) *jqlpb.Filter {
		return &jqlpb.Filter{Column: "Parent", Match: &jqlpb.Filter_PathToMatch{PathToMatch: &jqlpb.PathToMatch{Value: value, Reverse: reverse}}}
	}
	pkIn := func(vs ...string) *jqlpb.Filter {
		return &jqlpb.Filter{Column: "_Identifier", Match: &jqlpb.Filter_InMatch{InMatch: &jqlpb.InMatch{Values: vs}}}
	}
	pkEq := func(v string) *jqlpb.Filter {
		return &jqlpb.Filter{Column: "_Identifier", Match: &jqlpb.Filter_EqualMatch{EqualMatch: &jqlpb.EqualMatch{Value: v}}}
	}
	cases := [][]*jqlpb.Filter{
		{path("n0", false)},
		{path("n3", false)},
		{path("n150", false)},
		{path("missing", false)},
		{path("", false)},
		{path("n299", true)},
		{path("n42", true)},
		{path("missing", true)},
		{pkIn("n1", "n2", "missing")},
		{pkIn()},
		{pkEq("n7")},
		{pkEq("missing")},
		{path("n3", false), pkIn("n10", "n20", "n30")},
	}
	indexed := newTreeTable(t, 300, true)
	plain := newTreeTable(t, 300, false)
	for _, filters := range cases {
		want := queryTreePKs(t, plain, filters...)
		got := queryTreePKs(t, indexed, filters...)
		if fmt.Sprint(want) != fmt.Sprint(got) {
			t.Fatalf("results differ for %s:\nwant %v\ngot  %v", Description(filters[0]), want, got)
		}
	}
	// Sanity check that the traversals actually traverse
	if got := queryTreePKs(t, indexed, path("n0", false)); len(got) < 2 {
		t.Fatalf("expected descendants of n0, got %v", got)
	}
	if got := queryTreePKs(t, indexed, path("n299", true)); len(got) < 2 {
		t.Fatalf("expected ancestors of n299, got %v", got)
	}
}

func queryTreePKs(t testing.TB, table *types.Table, filters ...*jqlpb.Filter) []string {
	var shims []types.Filter
	for _, f := range filters {
		shims = append(shims, newFilterShim(f, table))
	}
	resp, err := table.Query(types.QueryParams{Filters: shims})
	if err != nil {
		t.Fatal(err)
	}
	var pks []string
	for _, row := range resp.Entries {
		pks = append(pks, row[table.Primary()].Format(""))
	}
	sort.Strings(pks)
	return pks
}

func benchmarkPath(b *testing.B, indexed bool) {
	table := newTreeTable(b, 100000, indexed)
	filter := &jqlpb.Filter{Column: "Parent", Match: &jqlpb.Filter_PathToMatch{PathToMatch: &jqlpb.PathToMatch{Value: "n5000"}}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		queryTreePKs(b, table, filter)
	}
}

func BenchmarkPathToMatchScan(b *testing.B)    { benchmarkPath(b, false) }
func BenchmarkPathToMatchIndexed(b *testing.B) { benchmarkPath(b, true) }
