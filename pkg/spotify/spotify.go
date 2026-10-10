package spotify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"spotapp-migrator/pkg/ui"
)

type Client struct {
	clientID     string
	clientSecret string
	redirectURI  string
	accessToken  string
	httpClient   *http.Client
	// localTracks, if non-nil, is used as the data source instead of the API
	// (CSV / Exportify mode). The map key is the playlist ID.
	localTracks map[string][]Track
}

type Playlist struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerName   string
	OwnerID     string
	TrackCount  int
}

type Track struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Artists    []string `json:"artists"`
	AlbumName  string   `json:"album"`
	ISRC       string   `json:"isrc"`
	DurationMs int      `json:"duration_ms"`
}

func NewClient(clientID, clientSecret, redirectURI string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// NewCSVClient creates a client that does not use the Spotify API, but data
// loaded from CSV files (Exportify). Useful when the app is in
// Development Mode and the API blocks access to tracks (403).
func NewCSVClient(dir string) (*Client, []Playlist, error) {
	playlists, tracksByID, err := LoadPlaylistsFromCSV(dir)
	if err != nil {
		return nil, nil, err
	}
	c := &Client{
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		localTracks: tracksByID,
	}
	return c, playlists, nil
}

// Authenticate runs local HTTP server and opens browser for Spotify OAuth2 authorization code flow.
func (c *Client) Authenticate() error {
	codeChan := make(chan string)
	errChan := make(chan error)

	u, err := url.Parse(c.redirectURI)
	if err != nil {
		return fmt.Errorf("invalid redirect URI: %w", err)
	}

	port := u.Port()
	if port == "" {
		port = "8080"
	}

	server := &http.Server{Addr: ":" + port}

	http.HandleFunc(u.Path, func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code != "" {
			fmt.Fprintln(w, "<html><body><h2>Authorization successful! You can close this tab and return to the terminal.</h2></body></html>")
			codeChan <- code
		} else {
			errStr := r.URL.Query().Get("error")
			fmt.Fprintf(w, "<html><body><h2>Authorization failed: %s</h2></body></html>", errStr)
			errChan <- fmt.Errorf("spotify auth error: %s", errStr)
		}
	})

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	authURL := fmt.Sprintf(
		"https://accounts.spotify.com/authorize?response_type=code&client_id=%s&scope=%s&redirect_uri=%s&show_dialog=true",
		url.QueryEscape(c.clientID),
		url.QueryEscape("playlist-read-private playlist-read-collaborative"),
		url.QueryEscape(c.redirectURI),
	)

	ui.LogInfo("Opening browser for Spotify authentication...")
	ui.LogInfo("If browser does not open automatically, visit this URL:\n\033[1;34m%s\033[0m\n", authURL)
	openBrowser(authURL)

	var code string
	select {
	case code = <-codeChan:
	case err := <-errChan:
		return err
	case <-time.After(3 * time.Minute):
		return fmt.Errorf("authentication timed out waiting for user response")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)

	return c.exchangeCode(code)
}

func (c *Client) exchangeCode(code string) error {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", c.redirectURI)

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	authHeader := base64.StdEncoding.EncodeToString([]byte(c.clientID + ":" + c.clientSecret))
	req.Header.Set("Authorization", "Basic "+authHeader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("spotify token exchange failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}

	c.accessToken = tokenResp.AccessToken
	return nil
}

// GetCurrentUserID returns the authenticated user's Spotify ID.
func (c *Client) GetCurrentUserID() (string, error) {
	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to fetch current user (status %d): %s", resp.StatusCode, string(body))
	}

	var me struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&me); err != nil {
		return "", err
	}
	return me.ID, nil
}

// GetUserPlaylists fetches all playlists for the authenticated user.
func (c *Client) GetUserPlaylists() ([]Playlist, error) {
	var playlists []Playlist
	nextURL := "https://api.spotify.com/v1/me/playlists?limit=50"

	for nextURL != "" {
		req, err := http.NewRequest("GET", nextURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.accessToken)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("failed to fetch spotify playlists (status %d): %s", resp.StatusCode, string(body))
		}

		var apiResp struct {
			Items []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				Tracks      struct {
					Total int `json:"total"`
				} `json:"tracks"`
				Owner struct {
					ID          string `json:"id"`
					DisplayName string `json:"display_name"`
				} `json:"owner"`
			} `json:"items"`
			Next string `json:"next"`
		}

		err = json.NewDecoder(resp.Body).Decode(&apiResp)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		for _, item := range apiResp.Items {
			playlists = append(playlists, Playlist{
				ID:          item.ID,
				Name:        item.Name,
				Description: item.Description,
				OwnerName:   item.Owner.DisplayName,
				OwnerID:     item.Owner.ID,
				TrackCount:  item.Tracks.Total,
			})
		}

		nextURL = apiResp.Next
	}

	return playlists, nil
}

// GetPlaylistTracks fetches all tracks for a given playlist.
func (c *Client) GetPlaylistTracks(playlistID string) ([]Track, error) {
	// CSV mode: return locally loaded tracks.
	if c.localTracks != nil {
		return c.localTracks[playlistID], nil
	}

	var tracks []Track
	nextURL := fmt.Sprintf("https://api.spotify.com/v1/playlists/%s/tracks?limit=100&market=from_token", playlistID)

	for nextURL != "" {
		req, err := http.NewRequest("GET", nextURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.accessToken)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
				return nil, fmt.Errorf("playlist not accessible (status %d): Spotify editorial playlist or restricted by app development mode", resp.StatusCode)
			}
			return nil, fmt.Errorf("failed to fetch tracks for playlist %s (status %d): %s", playlistID, resp.StatusCode, string(body))
		}

		var apiResp struct {
			Items []struct {
				Track struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Duration int    `json:"duration_ms"`
					Artists  []struct {
						Name string `json:"name"`
					} `json:"artists"`
					Album struct {
						Name string `json:"name"`
					} `json:"album"`
					ExternalIDs struct {
						ISRC string `json:"isrc"`
					} `json:"external_ids"`
				} `json:"track"`
			} `json:"items"`
			Next string `json:"next"`
		}

		err = json.NewDecoder(resp.Body).Decode(&apiResp)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		for _, item := range apiResp.Items {
			if item.Track.ID == "" && item.Track.Name == "" {
				continue // Skip local or unavailable tracks
			}

			artistNames := make([]string, 0, len(item.Track.Artists))
			for _, a := range item.Track.Artists {
				artistNames = append(artistNames, a.Name)
			}

			tracks = append(tracks, Track{
				ID:         item.Track.ID,
				Name:       item.Track.Name,
				Artists:    artistNames,
				AlbumName:  item.Track.Album.Name,
				ISRC:       item.Track.ExternalIDs.ISRC,
				DurationMs: item.Track.Duration,
			})
		}

		nextURL = apiResp.Next
	}

	return tracks, nil
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		// Ignore error, printed URL as fallback
	}
}
