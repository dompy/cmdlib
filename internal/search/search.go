package search

import (
	"sort"
	"strings"

	"github.com/dompy/cmdlib/internal/library"
)

// Filter requires every query word to match a case-insensitive subsequence.
// Exact substrings and name matches rank ahead of loose subsequences.
func Filter(cs []library.Command, query string) []library.Command {
	type hit struct {
		c     library.Command
		score int
	}
	hits := []hit{}
	for _, c := range cs {
		name := strings.ToLower(c.Name)
		hay := strings.ToLower(strings.Join([]string{c.Name, c.Command, c.Description, strings.Join(c.Tags, " "), c.Host, c.Risk}, " "))
		score := 0
		ok := true
		for _, word := range strings.Fields(strings.ToLower(query)) {
			if strings.Contains(name, word) {
				score += 100
				continue
			}
			if strings.Contains(hay, word) {
				score += 50
				continue
			}
			needle := []rune(word)
			matched := false
			for _, token := range strings.Fields(hay) {
				i := 0
				for _, r := range token {
					if i < len(needle) && r == needle[i] {
						i++
					}
				}
				if i == len(needle) {
					matched = true
					break
				}
			}
			if !matched {
				ok = false
				break
			}
			score++
		}
		if ok {
			hits = append(hits, hit{c, score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	out := make([]library.Command, len(hits))
	for i, h := range hits {
		out[i] = h.c
	}
	return out
}
