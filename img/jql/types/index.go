package types

import (
	"fmt"
	"regexp"
	"strings"
)

// Index types that may be configured on a column via the "index" key of its schema, e.g.
//
//	"assertions.Arg0": {"type": "string", "index": {"type": "exact"}}
//	"assertions.Arg1": {"type": "string", "index": {"type": "pattern", "pattern": "@\\{[^}]*\\}"}}
const (
	// IndexTypeExact maps the full user-input formatted value of an entry to the rows that hold it.
	// It accelerates equality (=) and membership (in) filters.
	IndexTypeExact = "exact"
	// IndexTypePattern maps every (lower-cased) substring of an entry matching a regular expression
	// to the rows that hold it. It accelerates case-insensitive substring (contains) filters, as well
	// as = and in filters, whose query value is itself a full match of the pattern.
	//
	// NOTE for correctness the pattern must be "self-delimiting": no match of the pattern may be
	// a proper prefix of another match starting at the same position. e.g. `@\{[^}]*\}` qualifies
	// because every match ends at the first '}' after its start.
	IndexTypePattern = "pattern"
)

// IndexSpec is the parsed configuration of a column index
type IndexSpec struct {
	Type    string
	Pattern string

	prefix   string         // literal prefix every pattern match must begin with
	anchored *regexp.Regexp // pattern anchored at the start of the input
	full     *regexp.Regexp // pattern anchored at both ends of the input
}

// ParseIndexSpec parses the "index" value of a column's schema
func ParseIndexSpec(raw interface{}) (*IndexSpec, error) {
	asMap, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("index must be an object, got %#v", raw)
	}
	spec := &IndexSpec{}
	if spec.Type, ok = asMap["type"].(string); !ok {
		return nil, fmt.Errorf("index is missing a string type: %#v", raw)
	}
	switch spec.Type {
	case IndexTypeExact:
	case IndexTypePattern:
		if spec.Pattern, ok = asMap["pattern"].(string); !ok || spec.Pattern == "" {
			return nil, fmt.Errorf("pattern index is missing a pattern: %#v", raw)
		}
		var err error
		if spec.anchored, err = regexp.Compile(`^(?:` + spec.Pattern + `)`); err != nil {
			return nil, fmt.Errorf("invalid index pattern %q: %s", spec.Pattern, err)
		}
		spec.full = regexp.MustCompile(`^(?:` + spec.Pattern + `)$`)
		spec.prefix, _ = spec.anchored.LiteralPrefix()
	default:
		return nil, fmt.Errorf("unknown index type: %s", spec.Type)
	}
	return spec, nil
}

// MatchKind is the semantics of a filter as far as an index is concerned
type MatchKind int

const (
	// MatchExact selects entries whose user-input format equals one of the hint's values
	MatchExact MatchKind = iota
	// MatchContains selects entries whose lower-cased canonical format contains the
	// lower-cased value of the hint
	MatchContains
)

// An IndexHint describes a filter in terms that an Index can use to find candidate rows
type IndexHint struct {
	Column int
	Kind   MatchKind
	Values []string
}

// A Hinter is a Filter that can describe itself to an index. Every row the Filter applies
// to must also satisfy the hint (a hint may be looser than its filter but never stricter).
type Hinter interface {
	IndexHint() (IndexHint, bool)
}

// An Index tracks the entries of a single column so that rows matching a filter
// can be found without a full table scan
type Index interface {
	Add(pk string, e Entry)
	Remove(pk string, e Entry)
	// Lookup returns a superset of the pks of rows satisfying the hint
	// or false if the index cannot serve the hint
	Lookup(h IndexHint) (map[string]struct{}, bool)
}

// NewIndex returns an empty index for the given spec
func NewIndex(spec *IndexSpec) Index {
	switch spec.Type {
	case IndexTypePattern:
		return &patternIndex{spec: spec, postings: postings{}}
	default:
		return &exactIndex{postings: postings{}}
	}
}

// postings maps an index key to the set of pks associated with it
type postings map[string]map[string]struct{}

func (p postings) add(key, pk string) {
	set, ok := p[key]
	if !ok {
		set = map[string]struct{}{}
		p[key] = set
	}
	set[pk] = struct{}{}
}

func (p postings) remove(key, pk string) {
	set := p[key]
	delete(set, pk)
	if len(set) == 0 {
		delete(p, key)
	}
}

// union returns the union of the posting sets of the given keys. When there is a single
// key the stored set is returned directly so callers must not mutate the result.
func (p postings) union(keys []string) map[string]struct{} {
	if len(keys) == 1 {
		return p[keys[0]]
	}
	ret := map[string]struct{}{}
	for _, key := range keys {
		for pk := range p[key] {
			ret[pk] = struct{}{}
		}
	}
	return ret
}

type exactIndex struct {
	postings postings
}

func (i *exactIndex) Add(pk string, e Entry) {
	i.postings.add(e.Format("user-input"), pk)
}

func (i *exactIndex) Remove(pk string, e Entry) {
	i.postings.remove(e.Format("user-input"), pk)
}

func (i *exactIndex) Lookup(h IndexHint) (map[string]struct{}, bool) {
	if h.Kind != MatchExact {
		return nil, false
	}
	return i.postings.union(h.Values), true
}

type patternIndex struct {
	spec     *IndexSpec
	postings postings
}

// tokens returns every substring of the lower-cased entry that is a match of the
// pattern, including overlapping matches
func (i *patternIndex) tokens(e Entry) map[string]struct{} {
	ret := map[string]struct{}{}
	formats := []string{e.Format("")}
	if ui := e.Format("user-input"); ui != formats[0] {
		formats = append(formats, ui)
	}
	for _, s := range formats {
		s = strings.ToLower(s)
		for start := 0; start <= len(s); start++ {
			if i.spec.prefix != "" {
				next := strings.Index(s[start:], i.spec.prefix)
				if next == -1 {
					break
				}
				start += next
			}
			if token := i.spec.anchored.FindString(s[start:]); token != "" {
				ret[token] = struct{}{}
			}
		}
	}
	return ret
}

func (i *patternIndex) Add(pk string, e Entry) {
	for token := range i.tokens(e) {
		i.postings.add(token, pk)
	}
}

func (i *patternIndex) Remove(pk string, e Entry) {
	for token := range i.tokens(e) {
		i.postings.remove(token, pk)
	}
}

func (i *patternIndex) Lookup(h IndexHint) (map[string]struct{}, bool) {
	keys := make([]string, 0, len(h.Values))
	for _, value := range h.Values {
		// A row containing (or equal to) the value has the value as a token so long as the
		// value is itself a full match of the (self-delimiting) pattern
		lowered := strings.ToLower(value)
		if lowered == "" || !i.spec.full.MatchString(lowered) {
			return nil, false
		}
		keys = append(keys, lowered)
	}
	return i.postings.union(keys), true
}
