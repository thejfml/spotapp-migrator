package migrator

import (
	"fmt"
	"strings"

	"spotapp-migrator/pkg/applemusic"
	"spotapp-migrator/pkg/spotify"
	"spotapp-migrator/pkg/ui"
)

type Engine struct {
	spotifyClient *spotify.Client
	appleClient   *applemusic.Client
	dryRun        bool
}

type MigrationResult struct {
	PlaylistName    string
	TotalTracks     int
	MatchedCount    int
	UnmatchedTracks []spotify.Track
	ApplePlaylistID string
}

func NewEngine(spotifyClient *spotify.Client, appleClient *applemusic.Client, dryRun bool) *Engine {
	return &Engine{
		spotifyClient: spotifyClient,
		appleClient:   appleClient,
		dryRun:        dryRun,
	}
}

// MigratePlaylist extracts tracks from Spotify, matches them on Apple Music, and creates the target playlist.
func (e *Engine) MigratePlaylist(spotifyPlaylist spotify.Playlist) (*MigrationResult, error) {
	ui.LogInfo("Fetching tracks for playlist: \033[1;36m%s\033[0m...", spotifyPlaylist.Name)
	tracks, err := e.spotifyClient.GetPlaylistTracks(spotifyPlaylist.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tracks from Spotify: %w", err)
	}

	result := &MigrationResult{
		PlaylistName:    spotifyPlaylist.Name,
		TotalTracks:     len(tracks),
		UnmatchedTracks: make([]spotify.Track, 0),
	}

	var matchedAppleIDs []string

	for idx, track := range tracks {
		primaryArtist := ""
		if len(track.Artists) > 0 {
			primaryArtist = track.Artists[0]
		}

		fmt.Printf("\r🔍 [%d/%d] Matching: %s - %s...", idx+1, len(tracks), track.Name, primaryArtist)

		var match *applemusic.SongMatch

		// 1. Try ISRC matching
		if track.ISRC != "" {
			m, err := e.appleClient.SearchTrackByISRC(track.ISRC)
			if err == nil && m != nil {
				match = m
			}
		}

		// 2. Fallback to Text Search
		if match == nil {
			m, err := e.appleClient.SearchTrackByQuery(track.Name, primaryArtist)
			if err == nil && m != nil {
				match = m
			}
		}

		if match != nil {
			matchedAppleIDs = append(matchedAppleIDs, match.ID)
			result.MatchedCount++
		} else {
			result.UnmatchedTracks = append(result.UnmatchedTracks, track)
		}
	}

	fmt.Println() // Clear line after search loop

	ui.LogInfo("Matched \033[1;32m%d/%d\033[0m tracks for playlist '%s'.", result.MatchedCount, result.TotalTracks, spotifyPlaylist.Name)

	if len(result.UnmatchedTracks) > 0 {
		ui.LogWarning("%d tracks could not be matched on Apple Music:", len(result.UnmatchedTracks))
		for _, u := range result.UnmatchedTracks {
			artists := strings.Join(u.Artists, ", ")
			fmt.Printf("   • %s - %s (ISRC: %s)\n", u.Name, artists, u.ISRC)
		}
	}

	if e.dryRun {
		ui.LogInfo("[DRY-RUN] Skipping Apple Music playlist creation for '%s'.", spotifyPlaylist.Name)
		return result, nil
	}

	if len(matchedAppleIDs) == 0 {
		ui.LogWarning("No tracks matched on Apple Music for playlist '%s'. Skipping playlist creation.", spotifyPlaylist.Name)
		return result, nil
	}

	ui.LogInfo("Creating playlist '%s' on Apple Music...", spotifyPlaylist.Name)
	applePlaylistID, err := e.appleClient.CreatePlaylist(spotifyPlaylist.Name, spotifyPlaylist.Description)
	if err != nil {
		return nil, fmt.Errorf("failed to create Apple Music playlist: %w", err)
	}

	result.ApplePlaylistID = applePlaylistID
	ui.LogSuccess("Created playlist on Apple Music (ID: %s)", applePlaylistID)

	ui.LogInfo("Adding %d tracks to Apple Music playlist...", len(matchedAppleIDs))
	if err := e.appleClient.AddTracksToPlaylist(applePlaylistID, matchedAppleIDs); err != nil {
		return nil, fmt.Errorf("failed to add tracks to Apple Music playlist: %w", err)
	}

	ui.LogSuccess("Successfully populated playlist '%s' on Apple Music!", spotifyPlaylist.Name)
	return result, nil
}
