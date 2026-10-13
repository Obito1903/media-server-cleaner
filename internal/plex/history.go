package plex

import (
	"context"
	"net/url"
	"path"
	"strconv"
	"time"
)

type HistoryEntry struct {
	Type                 string // "movie" or "episode"
	RatingKey            string
	ParentRatingKey      string
	GrandparentRatingKey string
	AccountID            int64
	User                 string
	ViewedAt             time.Time
}

const historyPageSize = 1000

// History returns the server-wide watch history ("viewed" events) for all accounts.
func (c *Client) History(ctx context.Context) ([]HistoryEntry, error) {
	var accounts struct {
		MediaContainer struct {
			Account []struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"Account"`
		} `json:"MediaContainer"`
	}
	if err := c.get(ctx, "/accounts", nil, &accounts); err != nil {
		return nil, err
	}
	names := map[int64]string{}
	for _, a := range accounts.MediaContainer.Account {
		names[a.ID] = a.Name
	}

	var out []HistoryEntry
	for start := 0; ; start += historyPageSize {
		var resp struct {
			MediaContainer struct {
				TotalSize int `json:"totalSize"`
				Metadata  []struct {
					Type           string `json:"type"`
					RatingKey      string `json:"ratingKey"`
					ParentKey      string `json:"parentKey"`
					GrandparentKey string `json:"grandparentKey"`
					ViewedAt       int64  `json:"viewedAt"`
					AccountID      int64  `json:"accountID"`
				} `json:"Metadata"`
			} `json:"MediaContainer"`
		}
		q := url.Values{
			"sort":                   {"viewedAt:desc"},
			"X-Plex-Container-Start": {strconv.Itoa(start)},
			"X-Plex-Container-Size":  {strconv.Itoa(historyPageSize)},
		}
		if err := c.get(ctx, "/status/sessions/history/all", q, &resp); err != nil {
			return nil, err
		}
		for _, m := range resp.MediaContainer.Metadata {
			user := names[m.AccountID]
			if user == "" {
				user = "account " + strconv.FormatInt(m.AccountID, 10)
			}
			e := HistoryEntry{
				Type:      m.Type,
				RatingKey: m.RatingKey,
				AccountID: m.AccountID,
				User:      user,
				ViewedAt:  time.Unix(m.ViewedAt, 0),
			}
			// Keys look like "/library/metadata/123".
			if m.ParentKey != "" {
				e.ParentRatingKey = path.Base(m.ParentKey)
			}
			if m.GrandparentKey != "" {
				e.GrandparentRatingKey = path.Base(m.GrandparentKey)
			}
			out = append(out, e)
		}
		n := len(resp.MediaContainer.Metadata)
		if n < historyPageSize || start+n >= resp.MediaContainer.TotalSize {
			return out, nil
		}
	}
}
