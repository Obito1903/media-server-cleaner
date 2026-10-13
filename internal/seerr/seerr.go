package seerr

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/obito1903/media-server-cleaner/internal/httpx"
)

type Client struct {
	base, apiKey string
}

func New(base, apiKey string) *Client {
	return &Client{base: base, apiKey: apiKey}
}

const (
	StatusPending  = 1
	StatusApproved = 2
	StatusDeclined = 3
)

type Request struct {
	ID          int       `json:"id"`
	Status      int       `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	Type        string    `json:"type"` // "movie" or "tv"
	RequestedBy struct {
		DisplayName  string `json:"displayName"`
		PlexUsername string `json:"plexUsername"`
		Username     string `json:"username"`
		Email        string `json:"email"`
	} `json:"requestedBy"`
	Seasons []struct {
		SeasonNumber int `json:"seasonNumber"`
	} `json:"seasons"`
	Media struct {
		TmdbID      int     `json:"tmdbId"`
		TvdbID      *int    `json:"tvdbId"`
		RatingKey   *string `json:"ratingKey"`
		RatingKey4k *string `json:"ratingKey4k"`
	} `json:"media"`
}

// User returns the best identifier to match against Plex/Tautulli usernames.
func (r Request) User() string {
	switch {
	case r.RequestedBy.PlexUsername != "":
		return r.RequestedBy.PlexUsername
	case r.RequestedBy.Username != "":
		return r.RequestedBy.Username
	case r.RequestedBy.DisplayName != "":
		return r.RequestedBy.DisplayName
	}
	return r.RequestedBy.Email
}

func (r Request) StatusString() string {
	switch r.Status {
	case StatusPending:
		return "pending"
	case StatusApproved:
		return "approved"
	case StatusDeclined:
		return "declined"
	}
	return "status " + strconv.Itoa(r.Status)
}

const pageSize = 100

func (c *Client) Requests(ctx context.Context) ([]Request, error) {
	var out []Request
	for skip := 0; ; skip += pageSize {
		var resp struct {
			PageInfo struct {
				Results int `json:"results"`
			} `json:"pageInfo"`
			Results []Request `json:"results"`
		}
		q := url.Values{
			"take":   {strconv.Itoa(pageSize)},
			"skip":   {strconv.Itoa(skip)},
			"filter": {"all"},
			"sort":   {"added"},
		}
		if err := httpx.GetJSON(ctx, c.base, "/api/v1/request", q, map[string]string{"X-Api-Key": c.apiKey}, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Results...)
		if len(resp.Results) < pageSize || len(out) >= resp.PageInfo.Results {
			return out, nil
		}
	}
}
