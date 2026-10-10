package applemusic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"spotapp-migrator/pkg/ui"
)

type Client struct {
	developerJWT string
	userToken    string
	storefront   string
	httpClient   *http.Client
}

type SongMatch struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	MatchedBy string
}

func NewClient(developerJWT, userToken, storefront string) *Client {
	if storefront == "" {
		storefront = "us"
	}
	return &Client{
		developerJWT: developerJWT,
		userToken:    userToken,
		storefront:   strings.ToLower(storefront),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchPublicDeveloperToken auto-scrapes Apple Music's public Developer JWT from music.apple.com JS assets.
func FetchPublicDeveloperToken() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequest("GET", "https://music.apple.com", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to reach music.apple.com: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Regex to find JS asset bundles
	reJS := regexp.MustCompile(`/assets/index-[a-zA-Z0-9_\-]+\.js`)
	matches := reJS.FindAllString(string(body), -1)

	if len(matches) == 0 {
		// Fallback regex for index or main JS files
		reJS = regexp.MustCompile(`/assets/[a-zA-Z0-9_\-]+\.js`)
		matches = reJS.FindAllString(string(body), -1)
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("could not find index JS assets on music.apple.com")
	}

	// Regex to detect JWT token format starting with eyJhbGciOi...
	reJWT := regexp.MustCompile(`eyJhbGciOiJFUzI1NiIs[a-zA-Z0-9_\-\.]+`)

	for _, jsPath := range matches {
		jsURL := "https://music.apple.com" + jsPath
		jsReq, err := http.NewRequest("GET", jsURL, nil)
		if err != nil {
			continue
		}
		jsReq.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")

		jsResp, err := client.Do(jsReq)
		if err != nil {
			continue
		}

		jsBody, err := io.ReadAll(jsResp.Body)
		jsResp.Body.Close()
		if err != nil {
			continue
		}

		jwtToken := reJWT.FindString(string(jsBody))
		if jwtToken != "" {
			return jwtToken, nil
		}
	}

	return "", fmt.Errorf("developer JWT token not found in music.apple.com JS bundles")
}

// EnsureDeveloperJWT checks if developer JWT is set; if empty, fetches automatically.
func (c *Client) EnsureDeveloperJWT() error {
	if c.developerJWT != "" {
		return nil
	}
	ui.LogInfo("Fetching Apple Music developer token from music.apple.com...")
	token, err := FetchPublicDeveloperToken()
	if err != nil {
		return fmt.Errorf("unable to auto-fetch Apple Developer JWT: %w", err)
	}
	c.developerJWT = token
	ui.LogSuccess("Obtained public Apple Music developer token!")
	return nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.developerJWT)
	if c.userToken != "" {
		req.Header.Set("Media-User-Token", c.userToken)
	}
	req.Header.Set("Origin", "https://music.apple.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
}

// SearchTrackByISRC searches Apple Music catalog by ISRC code.
func (c *Client) SearchTrackByISRC(isrc string) (*SongMatch, error) {
	if isrc == "" {
		return nil, nil
	}

	apiURL := fmt.Sprintf("https://api.music.apple.com/v1/catalog/%s/songs?filter[isrc]=%s", c.storefront, url.QueryEscape(isrc))
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil // Not found or error
	}

	var searchResp struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				Name       string `json:"name"`
				ArtistName string `json:"artistName"`
				AlbumName  string `json:"albumName"`
			} `json:"attributes"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	if len(searchResp.Data) > 0 {
		item := searchResp.Data[0]
		return &SongMatch{
			ID:        item.ID,
			Name:      item.Attributes.Name,
			Artist:    item.Attributes.ArtistName,
			Album:     item.Attributes.AlbumName,
			MatchedBy: "ISRC",
		}, nil
	}

	return nil, nil
}

// SearchTrackByQuery searches Apple Music catalog by track name and artist query.
func (c *Client) SearchTrackByQuery(trackName, primaryArtist string) (*SongMatch, error) {
	query := fmt.Sprintf("%s %s", trackName, primaryArtist)
	apiURL := fmt.Sprintf("https://api.music.apple.com/v1/catalog/%s/search?term=%s&types=songs&limit=5", c.storefront, url.QueryEscape(query))

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var searchResp struct {
		Results struct {
			Songs struct {
				Data []struct {
					ID         string `json:"id"`
					Attributes struct {
						Name       string `json:"name"`
						ArtistName string `json:"artistName"`
						AlbumName  string `json:"albumName"`
					} `json:"attributes"`
				} `json:"data"`
			} `json:"songs"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	songs := searchResp.Results.Songs.Data
	if len(songs) > 0 {
		item := songs[0]
		return &SongMatch{
			ID:        item.ID,
			Name:      item.Attributes.Name,
			Artist:    item.Attributes.ArtistName,
			Album:     item.Attributes.AlbumName,
			MatchedBy: "Text Query",
		}, nil
	}

	return nil, nil
}

// CreatePlaylist creates a new library playlist in Apple Music.
func (c *Client) CreatePlaylist(name, description string) (string, error) {
	if description == "" {
		description = "Migrated from Spotify via spotapp-migrator"
	}

	bodyData := map[string]interface{}{
		"attributes": map[string]string{
			"name":        name,
			"description": description,
		},
	}

	jsonBytes, err := json.Marshal(bodyData)
	if err != nil {
		return "", err
	}

	apiURL := "https://api.music.apple.com/v1/me/library/playlists"
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return "", err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to create Apple Music playlist (status %d): %s", resp.StatusCode, string(respBody))
	}

	var createResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBody, &createResp); err != nil {
		return "", err
	}

	if len(createResp.Data) == 0 {
		return "", fmt.Errorf("empty playlist creation response from Apple Music")
	}

	return createResp.Data[0].ID, nil
}

// AddTracksToPlaylist appends song IDs to an existing Apple Music library playlist in batches.
func (c *Client) AddTracksToPlaylist(playlistID string, trackIDs []string) error {
	if len(trackIDs) == 0 {
		return nil
	}

	// Apple Music API limits track insertion batch size (typically max 100 tracks per request)
	batchSize := 100
	for i := 0; i < len(trackIDs); i += batchSize {
		end := i + batchSize
		if end > len(trackIDs) {
			end = len(trackIDs)
		}
		chunk := trackIDs[i:end]

		dataItems := make([]map[string]string, 0, len(chunk))
		for _, id := range chunk {
			dataItems = append(dataItems, map[string]string{
				"id":   id,
				"type": "songs",
			})
		}

		bodyData := map[string]interface{}{
			"data": dataItems,
		}

		jsonBytes, err := json.Marshal(bodyData)
		if err != nil {
			return err
		}

		apiURL := fmt.Sprintf("https://api.music.apple.com/v1/me/library/playlists/%s/tracks", playlistID)
		req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBytes))
		if err != nil {
			return err
		}
		c.setHeaders(req)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("failed to add tracks to Apple Music playlist (status %d): %s", resp.StatusCode, string(respBody))
		}
		resp.Body.Close()
	}

	return nil
}
