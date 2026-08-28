// Package aia reads award data from AdultIndustryAwards, which exposes its
// performer pages through the WordPress REST API.
package aia

import (
	"regexp"
	"strings"
	"unicode"
)

// Slug derives the URL slug AdultIndustryAwards uses for a performer name:
// lowercase, punctuation dropped, runs of anything else collapsed to one hyphen.
func Slug(name string) string {
	var b strings.Builder
	lastHyphen := true // suppress a leading hyphen
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastHyphen = false
		case r == '\'' || r == '’' || r == '.':
			// Apostrophes and periods vanish rather than becoming separators, so
			// "D'Angelo" is "dangelo" and "J.J." is "jj".
		default:
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// awardLineRe pulls the leading year off a list item. Anything without one is a
// biography or promotional entry, not an award.
var awardLineRe = regexp.MustCompile(`^(\d{4})\s+(.+)$`)

// dashSplitRe finds the separators between an award name and the movie or credit
// details that follow. Only a dash with space around it counts, so hyphenated
// award names ("Best Three-Way Sex Scene") stay intact.
var dashSplitRe = regexp.MustCompile(`\s+[-\x{2013}\x{2014}]\s+`)

// splitAward separates the award name from the trailing details.
//
// The ADR called for splitting on the first dash, but award names contain their
// own qualifying dash ("Best Actress – Featurette – Games We Play, TrenchcoatX",
// "Best Three-Way Sex Scene – Girl/Girl/Boy – The Corruption Of Kissa Sins"), so
// the last separator is the one that divides name from details.
func splitAward(s string) (name, detail string) {
	matches := dashSplitRe.FindAllStringIndex(s, -1)
	if len(matches) == 0 {
		return strings.TrimSpace(s), ""
	}
	last := matches[len(matches)-1]
	return strings.TrimSpace(s[:last[0]]), strings.TrimSpace(s[last[1]:])
}

// splitOrganization takes the organization off the front of an award line. AIA
// writes it as a single token, optionally followed by the word "Awards"
// ("Pornhub Awards Nicest Tits"), which is dropped so one organization does not
// end up displayed as two.
func splitOrganization(s string) (org, rest string) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return "", ""
	}
	org = fields[0]
	fields = fields[1:]
	if len(fields) > 0 && strings.EqualFold(fields[0], "Awards") {
		fields = fields[1:]
	}
	return org, strings.Join(fields, " ")
}
