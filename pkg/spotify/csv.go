package spotify

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LoadPlaylistsFromCSV reads a directory containing CSV files exported from
// Exportify (https://exportify.net). Each .csv file represents a playlist,
// and its filename (without extension) is used as the playlist name.
//
// Returns the playlists alongside a map of playlistID -> tracks, where the ID
// matches the filename so they can be retrieved later.
func LoadPlaylistsFromCSV(dir string) ([]Playlist, map[string][]Track, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CSV directory '%s': %w", dir, err)
	}

	var playlists []Playlist
	tracksByID := make(map[string][]Track)

	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".csv") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		tracks, err := parseExportifyCSV(path)
		if err != nil {
			return nil, nil, fmt.Errorf("error reading '%s': %w", entry.Name(), err)
		}

		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		id := name // use filename as identifier

		playlists = append(playlists, Playlist{
			ID:         id,
			Name:       name,
			OwnerName:  "CSV",
			TrackCount: len(tracks),
		})
		tracksByID[id] = tracks
	}

	if len(playlists) == 0 {
		return nil, nil, fmt.Errorf("no .csv files found in '%s'", dir)
	}

	return playlists, tracksByID, nil
}

// parseExportifyCSV parses a single Exportify CSV file.
// Typical Exportify columns:
// "Track URI","Track Name","Artist Name(s)","Album Name","Album Artist Name(s)",
// "Album Release Date","Album Image URL","Disc Number","Track Number",
// "Track Duration (ms)","Track Preview URL","Explicit","Popularity","ISRC", ...
func parseExportifyCSV(path string) ([]Track, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1 // allow variable number of columns

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	col := indexColumns(header)

	var tracks []Track
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		name := get(record, col, "track name")
		if name == "" {
			continue // empty row or unavailable track
		}

		artists := splitArtists(get(record, col, "artist name(s)"))
		durationMs := 0
		if d := get(record, col, "track duration (ms)"); d != "" {
			durationMs, _ = strconv.Atoi(strings.TrimSpace(d))
		}

		tracks = append(tracks, Track{
			ID:         get(record, col, "track uri"),
			Name:       name,
			Artists:    artists,
			AlbumName:  get(record, col, "album name"),
			ISRC:       get(record, col, "isrc"),
			DurationMs: durationMs,
		})
	}

	return tracks, nil
}

// indexColumns builds a map of lowercase column-name -> index.
func indexColumns(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.ToLower(strings.TrimSpace(h))] = i
	}
	return m
}

// get returns the value of the specified column or "" if it does not exist.
func get(record []string, col map[string]int, name string) string {
	if idx, ok := col[name]; ok && idx < len(record) {
		return strings.TrimSpace(record[idx])
	}
	return ""
}

// splitArtists splits Exportify's artist field, which separates multiple artists with commas.
func splitArtists(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	artists := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			artists = append(artists, t)
		}
	}
	return artists
}
