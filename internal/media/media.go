package media

import (
	"fmt"
	"strings"
	"time"
)

type Item struct {
	RatingKey    string
	ParentKey    string // show rating key, for seasons
	Title        string // show title for seasons
	SeasonNumber int
	Year         int
	Type         string // "movie", "show" or "season"
	Library      string
	AddedAt      time.Time
	SizeBytes    int64
	LeafCount    int // episode count for shows/seasons
	GUIDs        []string

	// Watch stats (Tautulli)
	Plays      int
	PlexPlays  int            // subset of Plays taken from Plex history (pre-Tautulli)
	Users      map[string]int // user -> plays
	LastPlayed time.Time

	// Seerr
	Requests []Request

	// Scoring output
	RawScore  float64
	Score     float64
	Factors   []Factor
	Protected string // non-empty when protected by grace rules
}

type Request struct {
	User      string
	CreatedAt time.Time
	Status    string
}

type Factor struct {
	Name   string
	Points float64
	Max    float64
	Calc   string
}

// DisplayTitle includes the season for season items.
func (it *Item) DisplayTitle() string {
	if it.Type == "season" {
		if it.SeasonNumber == 0 {
			return it.Title + " · Specials"
		}
		return fmt.Sprintf("%s · S%02d", it.Title, it.SeasonNumber)
	}
	return it.Title
}

// RequesterWatched reports whether the given requester appears in the watch stats.
func (it *Item) RequesterWatched(user string) (int, bool) {
	for u, n := range it.Users {
		if strings.EqualFold(u, user) {
			return n, true
		}
	}
	return 0, false
}

func HumanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func DaysSince(t, now time.Time) float64 {
	return now.Sub(t).Hours() / 24
}
