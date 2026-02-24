package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/bilalbayram/metacli/internal/config"
)

type DiscoveredPage struct {
	PageID              string
	Name                string
	IGBusinessAccountID string
}

func (s *Service) DiscoverPages(ctx context.Context, profileName string) ([]DiscoveredPage, error) {
	return s.DiscoverPagesAndIGBusinessAccounts(ctx, profileName)
}

func (s *Service) DiscoverPagesAndIGBusinessAccounts(ctx context.Context, profileName string) ([]DiscoveredPage, error) {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return nil, err
	}
	_, profile, err := cfg.ResolveProfile(profileName)
	if err != nil {
		return nil, err
	}

	token, err := s.secrets.Get(profile.TokenRef)
	if err != nil {
		return nil, err
	}

	appSecret := ""
	if profile.AppSecretRef != "" {
		appSecret, err = s.secrets.Get(profile.AppSecretRef)
		if err != nil {
			return nil, err
		}
	}

	values := url.Values{}
	values.Set("fields", "id,name,instagram_business_account{id}")

	response := map[string]any{}
	if err := s.doRequest(ctx, http.MethodGet, profile.GraphVersion, "me/accounts", values, token, appSecret, &response); err != nil {
		return nil, err
	}

	data, ok := response["data"].([]any)
	if !ok {
		return nil, errors.New("discover pages response did not include data array")
	}

	pages := make([]DiscoveredPage, 0, len(data))
	for index, entry := range data {
		row, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("discover pages response contained invalid data[%d]", index)
		}

		pageID, _ := row["id"].(string)
		pageID = strings.TrimSpace(pageID)
		if pageID == "" {
			return nil, fmt.Errorf("discover pages response contained missing id at data[%d]", index)
		}

		name, _ := row["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("discover pages response contained missing name for page %q", pageID)
		}

		igBusinessID := ""
		if rawIG, ok := row["instagram_business_account"]; ok && rawIG != nil {
			igMap, ok := rawIG.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("discover pages response contained invalid instagram_business_account for page %q", pageID)
			}
			igBusinessID, _ = igMap["id"].(string)
			igBusinessID = strings.TrimSpace(igBusinessID)
			if igBusinessID == "" {
				return nil, fmt.Errorf("discover pages response contained blank instagram_business_account id for page %q", pageID)
			}
		}

		pages = append(pages, DiscoveredPage{
			PageID:              pageID,
			Name:                name,
			IGBusinessAccountID: igBusinessID,
		})
	}

	return pages, nil
}
