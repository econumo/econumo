// Package system reports instance-level information; currently the latest
// published Econumo release, polled from the econumo.com feed.
package system

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

const DefaultFeedURL = "https://econumo.com/releases/latest.json"

// The feed is remote input rendered as a trusted link in every instance's UI:
// only well-formed versions and econumo.com URLs are ever accepted.
const allowedURLPrefix = "https://econumo.com/"

var versionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

const pollInterval = 24 * time.Hour

type Service struct {
	enabled bool
	feedURL string
	client  *http.Client

	mu   sync.RWMutex
	info model.GetUpdateInfoResult
}

func NewService(enabled bool, feedURL string) *Service {
	return &Service{
		enabled: enabled,
		feedURL: feedURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// GetUpdateInfo returns the cached latest-release info; empty strings when the
// check is disabled, hasn't run yet, or nothing valid was ever received. The
// SPA does the version comparison, so failure here is silence, never an error.
func (s *Service) GetUpdateInfo(_ context.Context, _ vo.Id) (model.GetUpdateInfoResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.info, nil
}

// StartPolling launches the background feed poll (boot + every 24h). Only the
// serve command calls this; CLI commands and tests never do, so their
// responses stay deterministic and hermetic.
func (s *Service) StartPolling(ctx context.Context) {
	if !s.enabled {
		return
	}
	go func() {
		s.fetch(ctx)
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.fetch(ctx)
			}
		}
	}()
}

func (s *Service) fetch(ctx context.Context) {
	if err := s.fetchOnce(ctx); err != nil {
		slog.Debug("release feed check failed", "err", err)
	}
}

func (s *Service) fetchOnce(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.feedURL, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feed returned status %d", resp.StatusCode)
	}
	var feed struct {
		Version string `json:"version"`
		Url     string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&feed); err != nil {
		return err
	}
	if !versionPattern.MatchString(feed.Version) {
		return fmt.Errorf("feed version %q is not vX.Y.Z", feed.Version)
	}
	if !strings.HasPrefix(feed.Url, allowedURLPrefix) {
		return fmt.Errorf("feed url %q is outside %s", feed.Url, allowedURLPrefix)
	}
	s.mu.Lock()
	s.info = model.GetUpdateInfoResult{Version: feed.Version, Url: feed.Url}
	s.mu.Unlock()
	return nil
}
