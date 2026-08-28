package aia

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/slick-daddy/stash-awards/internal/store"
)

// minAwardYear rejects list items that happen to start with four digits but are
// not years.
const minAwardYear = 1900

// ParseAwards reads the rendered HTML of a performer post and returns the awards
// in it. Awards are plain list items, mixed in among biography facts and
// promotional links, so items are recognised by shape rather than by position.
func ParseAwards(r io.Reader, pageURL string) ([]store.Award, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse aia content: %w", err)
	}

	maxYear := time.Now().Year() + 2
	var out []store.Award
	doc.Find("li").Each(func(_ int, li *goquery.Selection) {
		if a, ok := parseLine(clean(li.Text()), maxYear); ok {
			a.SourceURL = pageURL
			out = append(out, a)
		}
	})
	return out, nil
}

// parseLine turns one list item's text into an award record.
//
// The shape is "YEAR ORG Award Name – Movie, Studio", where the dash and
// everything after it are optional and the site is inconsistent about including
// them.
func parseLine(text string, maxYear int) (store.Award, bool) {
	m := awardLineRe.FindStringSubmatch(text)
	if m == nil {
		return store.Award{}, false
	}
	year, err := strconv.Atoi(m[1])
	if err != nil || year < minAwardYear || year > maxYear {
		return store.Award{}, false
	}

	org, rest := splitOrganization(m[2])
	if org == "" || rest == "" {
		return store.Award{}, false
	}
	name, detail := splitAward(rest)
	if name == "" {
		return store.Award{}, false
	}

	award := store.Award{
		Organization: org,
		AwardName:    name,
		Year:         year,
		Event:        fmt.Sprintf("%s %d", org, year),
		// AdultIndustryAwards lists only wins; it carries no nominations.
		Result:          store.ResultWon,
		AssociatedMovie: detail,
	}
	if strings.Contains(strings.ToLower(name), "hall of fame") {
		award.Result = store.ResultInducted
		// A hall of fame entry has no film, so whatever followed the dash is part
		// of the honour's name rather than a credit.
		award.AwardName = strings.TrimSpace(name + " " + detail)
		award.AssociatedMovie = ""
	}
	return award, true
}

// clean collapses whitespace, including the non-breaking spaces WordPress leaves
// in rendered content.
func clean(s string) string {
	s = strings.ReplaceAll(s, " ", " ")
	return strings.Join(strings.Fields(s), " ")
}
