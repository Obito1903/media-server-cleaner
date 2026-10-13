package scoring

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/obito1903/media-server-cleaner/internal/media"
)

// Params controls the scoring. Higher final score = better deletion candidate.
type Params struct {
	WLastActivity float64
	WWatches      float64
	WViewers      float64
	WRequest      float64
	WSize         float64
	WAge          float64

	StaleDays float64 // days of inactivity/age considered "fully stale"
	GraceDays float64 // items added/requested/played within this window are protected
	SizeCapGB float64 // size at which the size factor maxes out
}

func DefaultParams() Params {
	return Params{
		WLastActivity: 30,
		WWatches:      20,
		WViewers:      15,
		WRequest:      15,
		WSize:         10,
		WAge:          10,
		StaleDays:     365,
		GraceDays:     30,
		SizeCapGB:     50,
	}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func Score(it *media.Item, p Params, now time.Time) {
	it.Factors = it.Factors[:0]
	add := func(name string, w, frac float64, calc string) {
		it.Factors = append(it.Factors, media.Factor{
			Name:   name,
			Points: w * frac,
			Max:    w,
			Calc:   fmt.Sprintf("%s → %.2f × %.0f", calc, frac, w),
		})
	}

	addedDays := media.DaysSince(it.AddedAt, now)

	// Last activity: time since last play, or since added if never played.
	if it.LastPlayed.IsZero() {
		add("Last activity", p.WLastActivity, clamp01(addedDays/p.StaleDays),
			fmt.Sprintf("never played, added %.0fd ago: min(%.0f/%.0f, 1)", addedDays, addedDays, p.StaleDays))
	} else {
		d := media.DaysSince(it.LastPlayed, now)
		add("Last activity", p.WLastActivity, clamp01(d/p.StaleDays),
			fmt.Sprintf("last played %.0fd ago: min(%.0f/%.0f, 1)", d, d, p.StaleDays))
	}

	// Watches: full-watch equivalents (episode plays / episode count for shows).
	watches := float64(it.Plays)
	calc := fmt.Sprintf("%d plays", it.Plays)
	if it.Type != "movie" {
		leaf := math.Max(1, float64(it.LeafCount))
		watches = float64(it.Plays) / leaf
		calc = fmt.Sprintf("%d episode plays / %d episodes = %.2f watches", it.Plays, it.LeafCount, watches)
	}
	add("Watches", p.WWatches, 1/(1+watches), calc+fmt.Sprintf(": 1/(1+%.2f)", watches))

	// Unique viewers.
	n := len(it.Users)
	add("Unique viewers", p.WViewers, 1/(1+float64(n)), fmt.Sprintf("%d viewers: 1/(1+%d)", n, n))

	// Requester engagement.
	if len(it.Requests) == 0 {
		add("Requester engagement", p.WRequest, 0.5, "not requested via Seerr (neutral)")
	} else {
		seen := map[string]bool{}
		var watched, total int
		var parts []string
		for _, r := range it.Requests {
			key := strings.ToLower(r.User)
			if seen[key] {
				continue
			}
			seen[key] = true
			total++
			if plays, ok := it.RequesterWatched(r.User); ok {
				watched++
				parts = append(parts, fmt.Sprintf("%s watched (%d)", r.User, plays))
			} else {
				parts = append(parts, r.User+" never watched")
			}
		}
		add("Requester engagement", p.WRequest, 1-float64(watched)/float64(total),
			fmt.Sprintf("%s: 1 - %d/%d", strings.Join(parts, ", "), watched, total))
	}

	// Size.
	gb := float64(it.SizeBytes) / (1 << 30)
	add("Size", p.WSize, clamp01(gb/p.SizeCapGB), fmt.Sprintf("%.1f GiB: min(%.1f/%.0f, 1)", gb, gb, p.SizeCapGB))

	// Age in library.
	add("Age in library", p.WAge, clamp01(addedDays/p.StaleDays),
		fmt.Sprintf("added %.0fd ago: min(%.0f/%.0f, 1)", addedDays, addedDays, p.StaleDays))

	var raw, max float64
	for _, f := range it.Factors {
		raw += f.Points
		max += f.Max
	}
	if max > 0 {
		raw = raw / max * 100
	}
	it.RawScore = raw
	it.Score = raw

	// Grace rules.
	it.Protected = ""
	var reasons []string
	if addedDays < p.GraceDays {
		reasons = append(reasons, fmt.Sprintf("added %.0fd ago", addedDays))
	}
	if !it.LastPlayed.IsZero() && media.DaysSince(it.LastPlayed, now) < p.GraceDays {
		reasons = append(reasons, fmt.Sprintf("played %.0fd ago", media.DaysSince(it.LastPlayed, now)))
	}
	for _, r := range it.Requests {
		if d := media.DaysSince(r.CreatedAt, now); d < p.GraceDays {
			reasons = append(reasons, fmt.Sprintf("requested by %s %.0fd ago", r.User, d))
			break
		}
	}
	if len(reasons) > 0 {
		it.Protected = fmt.Sprintf("%s (< %.0fd grace)", strings.Join(reasons, ", "), p.GraceDays)
		it.Score = 0
	}
}

func ScoreAll(items []*media.Item, p Params, now time.Time) {
	for _, it := range items {
		Score(it, p, now)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Score > items[j].Score })
}
