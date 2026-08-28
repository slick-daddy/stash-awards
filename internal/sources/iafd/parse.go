package iafd

import (
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/slick-daddy/stash-awards/internal/sources"
	"github.com/slick-daddy/stash-awards/internal/store"
)

// resultPrefixes maps the label IAFD puts in front of an entry onto a result.
// "Winner" and "Inducted" appear in bold, "Nominee" does not, but the label is
// the reliable signal and the bold formatting is only a fallback.
var resultPrefixes = map[string]store.Result{
	"winner":    store.ResultWon,
	"won":       store.ResultWon,
	"nominee":   store.ResultNominated,
	"nominated": store.ResultNominated,
	"inducted":  store.ResultInducted,
}

// movieYearRe matches the parenthesised release year IAFD puts after a movie
// link, e.g. " (2015)".
var movieYearRe = regexp.MustCompile(`\((\d{4})\)`)

// ParseAwards reads a performer page and returns every award in its awards tab.
//
// The awards panel is a flat run of siblings rather than nested markup, so the
// organization and year are carried forward as the run is walked:
//
//	<p class="bioheading">AVN Awards</p>
//	<div class="showyear">2016</div>
//	<div class="biodata"><b>Winner: Best Oral Sex Scene</b>, <a ...>Angela 2</a>&nbsp;(2015)</div>
func ParseAwards(r io.Reader, pageURL string) ([]store.Award, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse iafd page: %w", err)
	}

	panel := doc.Find("#awards")
	if panel.Length() == 0 {
		// A performer with no awards has no panel at all. That is not an error;
		// it is the answer.
		return nil, nil
	}

	base, _ := url.Parse(pageURL)

	var (
		out  []store.Award
		org  string
		year int
	)
	panel.First().Find("p.bioheading, div.showyear, div.biodata").Each(func(_ int, sel *goquery.Selection) {
		text := clean(sel.Text())
		switch {
		case sel.Is("p.bioheading"):
			org = text
			year = 0
		case sel.Is("div.showyear"):
			if n, err := strconv.Atoi(text); err == nil {
				year = n
			}
		case sel.Is("div.biodata"):
			if org == "" || year == 0 || text == "" {
				// An entry with no heading above it cannot be attributed, and a
				// year is part of the record's identity.
				return
			}
			if a, ok := parseEntry(sel, org, year, base, pageURL); ok {
				out = append(out, a)
			}
		}
	})
	return out, nil
}

// parseEntry turns one div.biodata into an award record.
func parseEntry(sel *goquery.Selection, org string, year int, base *url.URL, pageURL string) (store.Award, bool) {
	award := store.Award{
		Organization: org,
		Year:         year,
		Result:       store.ResultNominated,
		SourceURL:    pageURL,
	}

	// The movie is the only link in an entry. Splitting on it separates the award
	// name from the movie reliably, which a comma-split cannot do: award names
	// themselves contain commas and colons ("Best Three-Way Sex Scene: G/G/B").
	link := sel.Find("a").First()
	var nameText, tailText string
	if link.Length() > 0 {
		award.AssociatedMovie = clean(link.Text())
		if href, ok := link.Attr("href"); ok {
			award.AssociatedMovieURL = resolve(base, href)
		}
		nameText = clean(textBefore(sel, link))
		tailText = clean(textAfter(sel, link))
	} else {
		nameText = clean(sel.Text())
	}

	nameText = strings.TrimRight(nameText, " ,;")
	if nameText == "" {
		return store.Award{}, false
	}

	label, rest := splitLabel(nameText)
	if result, ok := resultPrefixes[strings.ToLower(label)]; ok {
		award.Result = result
		award.AwardName = rest
	} else {
		// No recognised label. Bold means a win on this site; anything else is
		// left at the nominated default.
		award.AwardName = nameText
		if sel.Find("b, strong").Length() > 0 {
			award.Result = store.ResultWon
		}
	}
	if award.AwardName == "" {
		return store.Award{}, false
	}

	if m := movieYearRe.FindStringSubmatch(tailText); m != nil {
		award.AssociatedMovieYear, _ = strconv.Atoi(m[1])
	}
	award.Event = fmt.Sprintf("%s %d", org, year)
	return award, true
}

// splitLabel splits "Winner: Best Oral Sex Scene" on the first colon only, since
// award names can contain further colons.
func splitLabel(s string) (label, rest string) {
	i := strings.Index(s, ":")
	if i < 0 {
		return "", s
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
}

// textBefore returns the text of every node preceding stop within sel.
func textBefore(sel, stop *goquery.Selection) string {
	var b strings.Builder
	collect(sel, stop, &b, false)
	return b.String()
}

// textAfter returns the text of every node following stop within sel.
func textAfter(sel, stop *goquery.Selection) string {
	var b strings.Builder
	collect(sel, stop, &b, true)
	return b.String()
}

// collect walks sel's descendants in document order, writing text either before
// or after the stop node. goquery has no "text up to this node" helper, so the
// walk is explicit.
func collect(sel, stop *goquery.Selection, b *strings.Builder, after bool) {
	stopNode := stop.Nodes[0]
	seen := false
	var walk func(*goquery.Selection)
	walk = func(s *goquery.Selection) {
		s.Contents().Each(func(_ int, child *goquery.Selection) {
			node := child.Nodes[0]
			if node == stopNode {
				seen = true
				return
			}
			if seen != after {
				return
			}
			if goquery.NodeName(child) == "#text" {
				b.WriteString(child.Text())
				return
			}
			walk(child)
		})
	}
	walk(sel)
}

// personLinkRe matches a performer page path, with or without a host.
var personLinkRe = regexp.MustCompile(`(?i)person\.rme/id=([0-9a-f-]{36})`)

// ParseSearch reads a comprehensive search results page and returns the performer
// matches. The results page has separate tables for female, male and trans
// performers with different ids, so rows are selected by what they link to rather
// than by which table they sit in.
func ParseSearch(r io.Reader, pageURL string) ([]sources.Match, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse iafd search results: %w", err)
	}
	base, _ := url.Parse(pageURL)

	var out []sources.Match
	seen := map[string]bool{}
	doc.Find("tr").Each(func(_ int, row *goquery.Selection) {
		// Both the "pretty" and the canonical id= form live under person.rme, and
		// only performer links do, so this one selector rejects the movie and
		// review tables without knowing their ids.
		link := row.Find(`a[href*="person.rme"]`).Last()
		if link.Length() == 0 {
			return
		}
		href, _ := link.Attr("href")
		target := resolve(base, href)
		if target == "" {
			return
		}
		// Result rows normally carry the id= form; a "pretty" URL is kept as-is
		// because IAFD redirects it to the canonical one when fetched.
		if canonical, ok := canonicalPersonURL(target); ok {
			target = canonical
		}
		if seen[target] {
			return
		}

		name := clean(link.Text())
		if name == "" {
			return
		}
		seen[target] = true

		m := sources.Match{Name: name, URL: target, Detail: searchDetail(row)}
		if src, ok := row.Find("img").First().Attr("src"); ok {
			m.ImageURL = resolve(base, src)
		}
		out = append(out, m)
	})
	return out, nil
}

// searchDetail summarises a result row's remaining columns: aliases and the years
// the performer was active.
func searchDetail(row *goquery.Selection) string {
	cells := row.Find("td")
	var aka, start, end string
	// Columns are headshot, name, AKA, start, end, titles.
	if cells.Length() >= 5 {
		aka = clean(cells.Eq(2).Text())
		start = clean(cells.Eq(3).Text())
		end = clean(cells.Eq(4).Text())
	}

	var parts []string
	if start != "" && end != "" {
		parts = append(parts, start+"–"+end)
	}
	if aka != "" {
		parts = append(parts, "aka "+aka)
	}
	return strings.Join(parts, " · ")
}

// resolve turns a possibly relative href into an absolute URL. IAFD is
// inconsistent about this: search results use relative paths while movie links on
// a performer page are absolute.
func resolve(base *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	if base == nil || ref.IsAbs() {
		return ref.String()
	}
	return base.ResolveReference(ref).String()
}

// clean collapses the whitespace IAFD pads its cells with, including the
// non-breaking spaces it uses before movie years.
func clean(s string) string {
	s = strings.ReplaceAll(s, " ", " ")
	return strings.Join(strings.Fields(s), " ")
}
