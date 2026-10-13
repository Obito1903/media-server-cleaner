package tautulli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/obito1903/media-server-cleaner/internal/httpx"
)

type Client struct {
	base, apiKey string
}

func New(base, apiKey string) *Client {
	return &Client{base: base, apiKey: apiKey}
}

// FlexString accepts JSON strings or numbers (Tautulli is inconsistent).
type FlexString string

func (f *FlexString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*f = ""
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = FlexString(s)
		return nil
	}
	*f = FlexString(b)
	return nil
}

func (f FlexString) Int() int64 {
	n, _ := strconv.ParseFloat(string(f), 64)
	return int64(n)
}

type HistoryEntry struct {
	Date                 FlexString `json:"date"`
	RatingKey            FlexString `json:"rating_key"`
	ParentRatingKey      FlexString `json:"parent_rating_key"`
	GrandparentRatingKey FlexString `json:"grandparent_rating_key"`
	User                 string     `json:"user"`
	FriendlyName         string     `json:"friendly_name"`
	MediaType            string     `json:"media_type"`
	PercentComplete      FlexString `json:"percent_complete"`
}

const pageSize = 5000

// Usernames maps Plex account IDs to Plex usernames.
func (c *Client) Usernames(ctx context.Context) (map[int64]string, error) {
	var resp struct {
		Response struct {
			Result  string `json:"result"`
			Message string `json:"message"`
			Data    []struct {
				UserID   FlexString `json:"user_id"`
				Username string     `json:"username"`
			} `json:"data"`
		} `json:"response"`
	}
	q := url.Values{"apikey": {c.apiKey}, "cmd": {"get_users"}}
	if err := httpx.GetJSON(ctx, c.base, "/api/v2", q, nil, &resp); err != nil {
		return nil, err
	}
	if resp.Response.Result != "success" {
		return nil, fmt.Errorf("tautulli get_users: %s", resp.Response.Message)
	}
	out := map[int64]string{}
	for _, u := range resp.Response.Data {
		if u.Username != "" {
			out[u.UserID.Int()] = u.Username
		}
	}
	return out, nil
}

// History returns the full play history.
func (c *Client) History(ctx context.Context) ([]HistoryEntry, error) {
	var out []HistoryEntry
	for start := 0; ; start += pageSize {
		var resp struct {
			Response struct {
				Result  string `json:"result"`
				Message string `json:"message"`
				Data    struct {
					RecordsFiltered int            `json:"recordsFiltered"`
					Data            []HistoryEntry `json:"data"`
				} `json:"data"`
			} `json:"response"`
		}
		q := url.Values{
			"apikey":       {c.apiKey},
			"cmd":          {"get_history"},
			"start":        {strconv.Itoa(start)},
			"length":       {strconv.Itoa(pageSize)},
			"order_column": {"date"},
			"order_dir":    {"desc"},
		}
		if err := httpx.GetJSON(ctx, c.base, "/api/v2", q, nil, &resp); err != nil {
			return nil, err
		}
		if resp.Response.Result != "success" {
			return nil, fmt.Errorf("tautulli get_history: %s", resp.Response.Message)
		}
		out = append(out, resp.Response.Data.Data...)
		if len(resp.Response.Data.Data) < pageSize || len(out) >= resp.Response.Data.RecordsFiltered {
			return out, nil
		}
	}
}
