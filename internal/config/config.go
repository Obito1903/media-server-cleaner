package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"github.com/obito1903/media-server-cleaner/internal/scoring"
)

type Config struct {
	PlexURL, PlexToken          string
	TautulliURL, TautulliAPIKey string
	SeerrURL, SeerrKey          string
	Libraries                   []string
	Scoring                     scoring.Params
}

func Load() (Config, error) {
	_ = godotenv.Load() // .env is optional; real env vars take precedence

	c := Config{
		PlexURL:        os.Getenv("PLEX_URL"),
		PlexToken:      os.Getenv("PLEX_TOKEN"),
		TautulliURL:    os.Getenv("TAUTULLI_URL"),
		TautulliAPIKey: os.Getenv("TAUTULLI_API_KEY"),
		SeerrURL:       os.Getenv("SEERR_URL"),
		SeerrKey:       os.Getenv("SEERR_API_KEY"),
		Scoring:        scoring.DefaultParams(),
	}
	if c.PlexURL == "" || c.PlexToken == "" {
		return c, errors.New("PLEX_URL and PLEX_TOKEN must be set (see .env.example)")
	}
	if v := os.Getenv("LIBRARIES"); v != "" {
		for _, l := range strings.Split(v, ",") {
			if l = strings.TrimSpace(l); l != "" {
				c.Libraries = append(c.Libraries, l)
			}
		}
	}

	floats := map[string]*float64{
		"WEIGHT_LAST_ACTIVITY": &c.Scoring.WLastActivity,
		"WEIGHT_WATCHES":       &c.Scoring.WWatches,
		"WEIGHT_VIEWERS":       &c.Scoring.WViewers,
		"WEIGHT_REQUEST":       &c.Scoring.WRequest,
		"WEIGHT_SIZE":          &c.Scoring.WSize,
		"WEIGHT_AGE":           &c.Scoring.WAge,
		"STALE_DAYS":           &c.Scoring.StaleDays,
		"GRACE_DAYS":           &c.Scoring.GraceDays,
		"SIZE_CAP_GB":          &c.Scoring.SizeCapGB,
	}
	for name, dst := range floats {
		v := os.Getenv(name)
		if v == "" {
			continue
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return c, fmt.Errorf("%s: %w", name, err)
		}
		*dst = f
	}
	return c, nil
}
