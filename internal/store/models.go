package store

// Source identifies where an award record came from.
type Source string

const (
	SourceIAFD Source = "iafd"
	SourceAIA  Source = "aia"
)

// Sources lists every supported source in display order.
var Sources = []Source{SourceIAFD, SourceAIA}

// Valid reports whether s is a source this plugin knows about. Anything arriving
// from the UI or a task argument has to be checked before it reaches a query.
func (s Source) Valid() bool {
	for _, known := range Sources {
		if s == known {
			return true
		}
	}
	return false
}

// Label is the human-readable source name shown in the UI.
func (s Source) Label() string {
	switch s {
	case SourceIAFD:
		return "IAFD"
	case SourceAIA:
		return "AdultIndustryAwards"
	}
	return string(s)
}

// Result is the outcome an award record represents.
type Result string

const (
	ResultWon       Result = "won"
	ResultNominated Result = "nominated"
	ResultInducted  Result = "inducted"
)

// Award is one award record for one performer.
type Award struct {
	ID           int64  `json:"id"`
	PerformerID  string `json:"performerId"`
	Source       Source `json:"source"`
	Organization string `json:"organization"`
	AwardName    string `json:"awardName"`
	Category     string `json:"category,omitempty"`
	Year         int    `json:"year"`
	Event        string `json:"event,omitempty"`
	Result       Result `json:"result"`
	SourceURL    string `json:"sourceUrl,omitempty"`

	// AssociatedMovie is the film or scene the award was given for; personal
	// awards have none. The URL and year come from the same link on the source
	// page and let the UI render "Angela 3 (2017)" as a link.
	AssociatedMovie     string `json:"associatedMovie,omitempty"`
	AssociatedMovieURL  string `json:"associatedMovieUrl,omitempty"`
	AssociatedMovieYear int    `json:"associatedMovieYear,omitempty"`

	LastScraped string `json:"lastScraped"`
}

// PerformerURL is a performer's resolved page on one source.
type PerformerURL struct {
	PerformerID string `json:"performerId"`
	Source      Source `json:"source"`
	URL         string `json:"url"`
	ResolvedAt  string `json:"resolvedAt"`
}

// SyncState records the outcome of the last sync for one performer and source.
type SyncState struct {
	PerformerID   string `json:"performerId"`
	Source        Source `json:"source"`
	LastSynced    string `json:"lastSynced,omitempty"`
	NextSyncAfter string `json:"nextSyncAfter,omitempty"`
	Error         string `json:"error,omitempty"`
}
