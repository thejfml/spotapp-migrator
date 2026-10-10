package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"spotapp-migrator/pkg/applemusic"
	"spotapp-migrator/pkg/config"
	"spotapp-migrator/pkg/migrator"
	"spotapp-migrator/pkg/spotify"
	"spotapp-migrator/pkg/ui"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "Simulate migration without creating Apple Music playlists")
	autoYes := flag.Bool("yes", false, "Automatically accept and migrate all playlists without prompting")
	ownOnly := flag.Bool("own-only", false, "Migrate only playlists owned by your account")
	fromCSV := flag.String("from-csv", "", "Directory containing CSV files exported from Exportify (bypasses Spotify API)")
	envPath := flag.String("config", ".env", "Path to configuration .env file")
	flag.Parse()

	fmt.Println(`
============================================================
   spotapp-migrator: Spotify to Apple Music Migration 
============================================================`)

	// 1. Load configuration
	cfg, err := config.LoadConfig(*envPath)
	if err != nil {
		ui.LogError("Failed to load config: %v", err)
		os.Exit(1)
	}

	// CSV Mode: Load playlists from exported CSV files (Exportify)
	// instead of using the Spotify API. Helps avoid 403 errors when apps
	// are in Development Mode.
	var spotifyClient *spotify.Client
	var playlists []spotify.Playlist
	csvMode := *fromCSV != ""

	if csvMode {
		ui.LogInfo("CSV mode enabled. Loading playlists from: %s", *fromCSV)
		spotifyClient, playlists, err = spotify.NewCSVClient(*fromCSV)
		if err != nil {
			ui.LogError("Failed to load CSV files: %v", err)
			os.Exit(1)
		}
		ui.LogSuccess("Loaded %d playlists from CSV.", len(playlists))
	}

	// 2. Validate / Prompt for Spotify credentials (only if not CSV mode)
	if !csvMode {
		if cfg.SpotifyClientID == "" {
			ui.LogWarning("SPOTIFY_CLIENT_ID not found in environment or .env file.")
			cfg.SpotifyClientID = ui.PromptInput("Enter your Spotify Client ID")
		}
		if cfg.SpotifyClientSecret == "" {
			ui.LogWarning("SPOTIFY_CLIENT_SECRET not found in environment or .env file.")
			cfg.SpotifyClientSecret = ui.PromptInput("Enter your Spotify Client Secret")
		}

		if cfg.SpotifyClientID == "" || cfg.SpotifyClientSecret == "" {
			ui.LogError("Spotify credentials are required to proceed.")
			os.Exit(1)
		}
	}

	// 3. Validate / Prompt for Apple Music credentials (if not dry run)
	if !*dryRun && cfg.AppleMusicUserToken == "" {
		ui.LogWarning("APPLE_MUSIC_USER_TOKEN not set.")
		ui.LogInfo("To obtain your media-user-token:")
		ui.LogInfo("  1. Open https://music.apple.com and log in to your Apple account.")
		ui.LogInfo("  2. Open Browser DevTools (F12) -> Application tab -> Cookies / LocalStorage.")
		ui.LogInfo("  3. Find and copy the value for key 'media-user-token'.\n")

		cfg.AppleMusicUserToken = ui.PromptInput("Enter your Apple Music media-user-token")
		if cfg.AppleMusicUserToken == "" {
			ui.LogError("Apple Music User Token is required unless running in --dry-run mode.")
			os.Exit(1)
		}
	}

	// 4. Initialize Clients
	if !csvMode {
		spotifyClient = spotify.NewClient(cfg.SpotifyClientID, cfg.SpotifyClientSecret, cfg.SpotifyRedirectURI)
		if err := spotifyClient.Authenticate(); err != nil {
			ui.LogError("Spotify authentication failed: %v", err)
			os.Exit(1)
		}
		ui.LogSuccess("Authenticated with Spotify!")
	}

	appleClient := applemusic.NewClient(cfg.AppleDeveloperJWT, cfg.AppleMusicUserToken, cfg.AppleStorefront)
	if err := appleClient.EnsureDeveloperJWT(); err != nil {
		ui.LogError("Apple Music setup failed: %v", err)
		os.Exit(1)
	}

	// 5. Fetch Spotify Playlists (API mode only; CSV mode already loaded playlists)
	if !csvMode {
		ui.LogInfo("Fetching Spotify playlists...")
		playlists, err = spotifyClient.GetUserPlaylists()
		if err != nil {
			ui.LogError("Failed to retrieve playlists: %v", err)
			os.Exit(1)
		}

		if len(playlists) == 0 {
			ui.LogWarning("No playlists found on your Spotify account.")
			return
		}

		ui.LogSuccess("Found %d playlists on Spotify.", len(playlists))

		// Filter playlists: always skip Spotify editorial playlists,
		// and if --own-only is set, keep only user-owned playlists.
		myID, err := spotifyClient.GetCurrentUserID()
		if err != nil {
			ui.LogWarning("Could not retrieve your Spotify user ID: %v", err)
		}

		var filtered []spotify.Playlist
		skippedSpotify := 0
		skippedOthers := 0
		for _, pl := range playlists {
			if pl.OwnerID == "spotify" {
				skippedSpotify++
				continue
			}
			if *ownOnly && myID != "" && pl.OwnerID != myID {
				skippedOthers++
				continue
			}
			filtered = append(filtered, pl)
		}
		playlists = filtered

		ui.LogInfo("Filtered out %d Spotify editorial playlists.", skippedSpotify)
		if *ownOnly {
			ui.LogInfo("Filtered out %d playlists from other users (--own-only).", skippedOthers)
		}
		ui.LogSuccess("Playlists to process: %d", len(playlists))
	}

	if len(playlists) == 0 {
		ui.LogWarning("No playlists to process after applying filters.")
		return
	}

	// 6. Interactive Migration Loop
	engine := migrator.NewEngine(spotifyClient, appleClient, *dryRun)
	autoAll := *autoYes

	var summary []*migrator.MigrationResult

	for _, pl := range playlists {
		if !autoAll {
			choice, err := ui.PromptPlaylist(pl.Name, pl.TrackCount, pl.OwnerName)
			if err != nil || choice == ui.ChoiceQuit {
				ui.LogInfo("Migration stopped by user.")
				break
			}

			if choice == ui.ChoiceNo {
				ui.LogInfo("Skipping playlist '%s'.", pl.Name)
				continue
			}

			if choice == ui.ChoiceAll {
				autoAll = true
				ui.LogInfo("Migrating all remaining playlists automatically...")
			}
		}

		result, err := engine.MigratePlaylist(pl)
		if err != nil {
			ui.LogError("Error migrating playlist '%s': %v", pl.Name, err)
			continue
		}
		summary = append(summary, result)
	}

	// 7. Summary Report
	fmt.Println("\n============================================================")
	fmt.Println("              MIGRATION SUMMARY REPORT ")
	fmt.Println("============================================================")
	totalMatched := 0
	totalTracks := 0

	for _, res := range summary {
		totalMatched += res.MatchedCount
		totalTracks += res.TotalTracks
		fmt.Printf("• %s: %d/%d tracks matched\n", res.PlaylistName, res.MatchedCount, res.TotalTracks)
	}

	if totalTracks > 0 {
		fmt.Printf("\nOverall Success Rate: %.1f%% (%d/%d tracks matched)\n", float64(totalMatched)/float64(totalTracks)*100, totalMatched, totalTracks)
	}
	fmt.Println("============================================================")

	if *dryRun {
		ui.LogInfo("[DRY-RUN COMPLETE] No changes were made to Apple Music.")
	} else {
		ui.LogSuccess("Migration complete!")
	}
}

func getAbsPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
