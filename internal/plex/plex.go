package plex

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/obito1903/media-server-cleaner/internal/httpx"
	"github.com/obito1903/media-server-cleaner/internal/media"
)

type Client struct {
	base, token string
}

func New(base, token string) *Client {
	return &Client{base: base, token: token}
}

type metadata struct {
	RatingKey            string `json:"ratingKey"`
	ParentRatingKey      string `json:"parentRatingKey"`
	GrandparentRatingKey string `json:"grandparentRatingKey"`
	Index                int    `json:"index"`
	Title                string `json:"title"`
	Type                 string `json:"type"`
	Year                 int    `json:"year"`
	AddedAt              int64  `json:"addedAt"`
	LeafCount            int    `json:"leafCount"`
	// Plex sends both "guid" (string) and "Guid" (array); this catches the former so it isn't matched case-insensitively to Guid.
	PlexGUID string `json:"guid"`
	Guid     []struct {
		ID string `json:"id"`
	} `json:"Guid"`
	Media []struct {
		Part []struct {
			Size int64 `json:"size"`
		} `json:"Part"`
	} `json:"Media"`
}

func (m metadata) size() int64 {
	var s int64
	for _, md := range m.Media {
		for _, p := range md.Part {
			s += p.Size
		}
	}
	return s
}

type metadataContainer struct {
	MediaContainer struct {
		Metadata []metadata `json:"Metadata"`
	} `json:"MediaContainer"`
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	return httpx.GetJSON(ctx, c.base, path, q, map[string]string{"X-Plex-Token": c.token}, out)
}

// Items returns all movies, shows and seasons from movie/show libraries.
// If only is non-empty, only libraries whose title matches (case-insensitive) are included.
func (c *Client) Items(ctx context.Context, only []string) ([]*media.Item, error) {
	var sections struct {
		MediaContainer struct {
			Directory []struct {
				Key   string `json:"key"`
				Type  string `json:"type"`
				Title string `json:"title"`
			} `json:"Directory"`
		} `json:"MediaContainer"`
	}
	if err := c.get(ctx, "/library/sections", nil, &sections); err != nil {
		return nil, err
	}

	var items []*media.Item
	for _, sec := range sections.MediaContainer.Directory {
		if sec.Type != "movie" && sec.Type != "show" {
			continue
		}
		if len(only) > 0 && !containsFold(only, sec.Title) {
			continue
		}
		var all metadataContainer
		if err := c.get(ctx, "/library/sections/"+sec.Key+"/all", url.Values{"includeGuids": {"1"}}, &all); err != nil {
			return nil, err
		}

		// Sizes keyed by show and by season rating key.
		sizes := map[string]int64{}
		var seasons metadataContainer
		if sec.Type == "show" {
			var eps metadataContainer
			if err := c.get(ctx, "/library/sections/"+sec.Key+"/all", url.Values{"type": {"4"}}, &eps); err != nil {
				return nil, err
			}
			for _, e := range eps.MediaContainer.Metadata {
				sizes[e.GrandparentRatingKey] += e.size()
				sizes[e.ParentRatingKey] += e.size()
			}
			if err := c.get(ctx, "/library/sections/"+sec.Key+"/all", url.Values{"type": {"3"}}, &seasons); err != nil {
				return nil, err
			}
		}

		shows := map[string]*media.Item{}
		for _, md := range all.MediaContainer.Metadata {
			it := &media.Item{
				RatingKey: md.RatingKey,
				Title:     md.Title,
				Year:      md.Year,
				Type:      md.Type,
				Library:   sec.Title,
				AddedAt:   time.Unix(md.AddedAt, 0),
				LeafCount: md.LeafCount,
				Users:     map[string]int{},
			}
			for _, g := range md.Guid {
				it.GUIDs = append(it.GUIDs, g.ID)
			}
			if md.Type == "show" {
				it.SizeBytes = sizes[md.RatingKey]
				shows[md.RatingKey] = it
			} else {
				it.SizeBytes = md.size()
			}
			items = append(items, it)
		}

		for _, md := range seasons.MediaContainer.Metadata {
			show := shows[md.ParentRatingKey]
			if show == nil {
				continue
			}
			items = append(items, &media.Item{
				RatingKey:    md.RatingKey,
				ParentKey:    show.RatingKey,
				Title:        show.Title,
				SeasonNumber: md.Index,
				Year:         show.Year,
				Type:         "season",
				Library:      sec.Title,
				AddedAt:      time.Unix(md.AddedAt, 0),
				LeafCount:    md.LeafCount,
				SizeBytes:    sizes[md.RatingKey],
				Users:        map[string]int{},
			})
		}
	}
	return items, nil
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(strings.TrimSpace(v), s) {
			return true
		}
	}
	return false
}
