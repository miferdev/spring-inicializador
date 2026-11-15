package tui

// Search ranking for the dependency list (US3.1).
//
// The brief fixes the field order: an id hit outranks a display-name hit, which
// outranks a category hit, so `postgres` finds the `postgresql` driver whose id
// nobody remembers. It lives here, next to no widgets, because it is a pure
// function over catalog values and is tested that way.

import (
	"sort"
	"strings"

	"github.com/miferdev/spring-inicializador/internal/catalog"
)

// matchField says where the term was found. The values are ordered by the
// weight the story gives them: lower is better.
type matchField int

const (
	matchedID matchField = iota
	matchedName
	matchedCategory
)

// matchQuality says how well that field answers the term. An exact hit is the
// best there is, then a hit at the start of the field, then one anywhere in it.
type matchQuality int

const (
	qualityExact matchQuality = iota
	qualityPrefix
	qualitySubstring
)

// dependencyMatch is one dependency that matched, with the reason, so the
// results can be ordered by how well they answer the query.
type dependencyMatch struct {
	index   int // position in the catalog's flat dependency slice
	field   matchField
	quality matchQuality
	width   int // length of the field that matched: the shorter, the more specific
}

// rankDependencies returns the positions of the dependencies that match term,
// best first. Positions index deps, so a caller that built its rows from the
// same slice can show the results without copying values around.
//
// The order is the story's: id hits before name hits before category hits, then
// quality (exact, prefix, substring), then the shorter field, then catalog
// order, which is the server's and is never sorted (decision D7 belongs to the
// UI, and this is the UI's search).
//
// A blank term is not a search: every dependency comes back in catalog order.
func rankDependencies(term string, deps []catalog.Dependency) []int {
	term = strings.ToLower(strings.TrimSpace(term))

	if term == "" {
		positions := make([]int, 0, len(deps))
		for i := range deps {
			positions = append(positions, i)
		}
		return positions
	}

	matches := make([]dependencyMatch, 0, len(deps))
	for i, dep := range deps {
		if match, ok := matchDependency(term, i, dep); ok {
			matches = append(matches, match)
		}
	}

	sort.Slice(matches, func(a, b int) bool {
		left, right := matches[a], matches[b]
		switch {
		case left.field != right.field:
			return left.field < right.field
		case left.quality != right.quality:
			return left.quality < right.quality
		case left.width != right.width:
			return left.width < right.width
		default:
			return left.index < right.index
		}
	})

	positions := make([]int, 0, len(matches))
	for _, match := range matches {
		positions = append(positions, match.index)
	}
	return positions
}

// matchDependency reports the strongest field of dep that answers term. The
// fields are tried in weight order, so a name that matches exactly still loses
// to an id that only contains the term: the brief fixes the field order, not the
// quality order across fields.
func matchDependency(term string, index int, dep catalog.Dependency) (dependencyMatch, bool) {
	fields := [...]struct {
		field matchField
		value string
	}{
		{matchedID, dep.ID},
		{matchedName, dep.Name},
		{matchedCategory, dep.Category},
	}

	for _, candidate := range fields {
		value := strings.ToLower(candidate.value)
		quality, ok := matchQualityOf(value, term)
		if !ok {
			continue
		}
		return dependencyMatch{
			index:   index,
			field:   candidate.field,
			quality: quality,
			width:   len(value),
		}, true
	}
	return dependencyMatch{}, false
}

// matchQualityOf grades how well candidate answers term, which is already
// lowercased and trimmed.
func matchQualityOf(candidate, term string) (matchQuality, bool) {
	switch {
	case candidate == term:
		return qualityExact, true
	case strings.HasPrefix(candidate, term):
		return qualityPrefix, true
	case strings.Contains(candidate, term):
		return qualitySubstring, true
	default:
		return qualityExact, false
	}
}
