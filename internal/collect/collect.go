package collect

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/obito1903/media-server-cleaner/internal/config"
	"github.com/obito1903/media-server-cleaner/internal/media"
	"github.com/obito1903/media-server-cleaner/internal/plex"
	"github.com/obito1903/media-server-cleaner/internal/seerr"
	"github.com/obito1903/media-server-cleaner/internal/tautulli"
)

// Collect fetches data from all services and merges it onto Plex items.
// Tautulli/Seerr failures are non-fatal and returned as warnings.
func Collect(ctx context.Context, cfg config.Config) ([]*media.Item, []string, error) {
	var (
		wg               sync.WaitGroup
		items            []*media.Item
		history          []tautulli.HistoryEntry
		usernames        map[int64]string
		plexHistory      []plex.HistoryEntry
		requests         []seerr.Request
		perr, terr, oerr error
		pherr            error
	)
	px := plex.New(cfg.PlexURL, cfg.PlexToken)
	wg.Add(4)
	go func() {
		defer wg.Done()
		items, perr = px.Items(ctx, cfg.Libraries)
	}()
	go func() {
		defer wg.Done()
		plexHistory, pherr = px.History(ctx)
	}()
	go func() {
		defer wg.Done()
		if cfg.TautulliURL == "" || cfg.TautulliAPIKey == "" {
			terr = fmt.Errorf("not configured")
			return
		}
		tc := tautulli.New(cfg.TautulliURL, cfg.TautulliAPIKey)
		if history, terr = tc.History(ctx); terr != nil {
			return
		}
		usernames, terr = tc.Usernames(ctx)
	}()
	go func() {
		defer wg.Done()
		if cfg.SeerrURL == "" || cfg.SeerrKey == "" {
			oerr = fmt.Errorf("not configured")
			return
		}
		requests, oerr = seerr.New(cfg.SeerrURL, cfg.SeerrKey).Requests(ctx)
	}()
	wg.Wait()

	if perr != nil {
		return nil, nil, fmt.Errorf("plex: %w", perr)
	}
	var warnings []string
	if terr != nil {
		warnings = append(warnings, "Tautulli: "+terr.Error())
	}
	if pherr != nil {
		warnings = append(warnings, "Plex history: "+pherr.Error())
	}
	if oerr != nil {
		warnings = append(warnings, "Seerr: "+oerr.Error())
	}

	byKey := map[string]*media.Item{}
	byGUID := map[string]*media.Item{}
	seasonsOf := map[string][]*media.Item{}
	for _, it := range items {
		byKey[it.RatingKey] = it
		for _, g := range it.GUIDs {
			byGUID[it.Type+":"+g] = it
		}
		if it.Type == "season" {
			seasonsOf[it.ParentKey] = append(seasonsOf[it.ParentKey], it)
		}
	}

	addPlay := func(keys []string, user string, t time.Time, fromPlex bool) {
		for _, key := range keys {
			it := byKey[key]
			if it == nil {
				continue
			}
			it.Plays++
			if fromPlex {
				it.PlexPlays++
			}
			it.Users[user]++
			if t.After(it.LastPlayed) {
				it.LastPlayed = t
			}
		}
	}

	// Plex history only fills the period before Tautulli started recording, to avoid double counting.
	cutoff := time.Now().Add(time.Hour)
	for _, h := range history {
		user := h.User
		if user == "" {
			user = h.FriendlyName
		}
		t := time.Unix(h.Date.Int(), 0)
		if t.Before(cutoff) {
			cutoff = t
		}
		keys := []string{string(h.RatingKey)}
		if h.MediaType == "episode" {
			keys = []string{string(h.GrandparentRatingKey), string(h.ParentRatingKey)}
		}
		addPlay(keys, user, t, false)
	}
	for _, h := range plexHistory {
		if !h.ViewedAt.Before(cutoff) {
			continue
		}
		keys := []string{h.RatingKey}
		if h.Type == "episode" {
			keys = []string{h.GrandparentRatingKey, h.ParentRatingKey}
		}
		user := h.User
		// Plex history has display names; prefer the username so it matches Tautulli/Seerr.
		if u := usernames[h.AccountID]; u != "" {
			user = u
		}
		addPlay(keys, user, h.ViewedAt, true)
	}

	for _, r := range requests {
		if r.Status == seerr.StatusDeclined {
			continue
		}
		it := matchRequest(r, byKey, byGUID)
		if it == nil {
			continue
		}
		req := media.Request{User: r.User(), CreatedAt: r.CreatedAt, Status: r.StatusString()}
		it.Requests = append(it.Requests, req)
		if it.Type != "show" {
			continue
		}
		requested := map[int]bool{}
		for _, s := range r.Seasons {
			requested[s.SeasonNumber] = true
		}
		for _, s := range seasonsOf[it.RatingKey] {
			if len(requested) == 0 || requested[s.SeasonNumber] {
				s.Requests = append(s.Requests, req)
			}
		}
	}
	return items, warnings, nil
}

func matchRequest(r seerr.Request, byKey, byGUID map[string]*media.Item) *media.Item {
	for _, k := range []*string{r.Media.RatingKey, r.Media.RatingKey4k} {
		if k != nil && byKey[*k] != nil {
			return byKey[*k]
		}
	}
	if r.Type == "movie" {
		return byGUID["movie:tmdb://"+strconv.Itoa(r.Media.TmdbID)]
	}
	if r.Media.TvdbID != nil {
		if it := byGUID["show:tvdb://"+strconv.Itoa(*r.Media.TvdbID)]; it != nil {
			return it
		}
	}
	return byGUID["show:tmdb://"+strconv.Itoa(r.Media.TmdbID)]
}
