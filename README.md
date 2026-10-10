# spotapp-migrator 🎵

`spotapp-migrator` is an interactive Go CLI tool to migrate playlists from **Spotify** to **Apple Music**.

---

## ⚡ Features

- **Interactive Selection**: Interactively prompts for each playlist (`[y]es / [n]o / [a]ll / [q]uit`).
- **High Matching Accuracy**: Matches songs using **ISRC** (International Standard Recording Code) first, falling back to track title and primary artist search.
- **Zero-Cost Apple Music API**: Auto-fetches the public Apple Music Developer JWT from `music.apple.com`, so **no $99/year Apple Developer license is needed**.
- **CSV Export Mode**: Load playlists from [Exportify](https://exportify.net) `.csv` files to bypass Spotify API restrictions and 403 errors (`--from-csv`).
- **Dry-Run Mode**: Test and preview playlist matching without creating playlists on Apple Music (`--dry-run`).

---

## 🚀 Quick Start

### 1. Requirements & Setup

Create a `.env` file in the project folder (or copy from `.env.example`):

```bash
cp .env.example .env
```

#### Spotify Setup (Free)
1. Go to [Spotify Developer Dashboard](https://developer.spotify.com/dashboard).
2. Create an App and get your **Client ID** & **Client Secret**.
3. Add `http://localhost:8080/callback` as a **Redirect URI** in your app settings.
4. Add credentials to `.env`:
   ```env
   SPOTIFY_CLIENT_ID=your_client_id
   SPOTIFY_CLIENT_SECRET=your_client_secret
   SPOTIFY_REDIRECT_URI=http://localhost:8080/callback
   ```

#### Alternative: CSV Mode (Bypass Spotify API)
If your Spotify developer app is restricted by **Development Mode** (causing 403 Forbidden errors when fetching tracks), you can export your playlists as CSV files:
1. Visit [https://exportify.net](https://exportify.net) and log in with your Spotify account.
2. Export your playlists to `.csv` files and save them in a directory (e.g. `./export`).
3. Run `spotapp-migrator` using the `--from-csv` flag:
   ```bash
   ./spotapp-migrator --from-csv ./export
   ```

#### Apple Music Setup (Free)
1. Open [music.apple.com](https://music.apple.com) in your browser and sign in to your Apple Music account.
2. Open Developer Tools (`F12` or Right Click -> Inspect).
3. Obtain **Media User Token**:
   - Go to **Application** (or **Storage**) -> **Cookies** / **Local Storage**.
   - Find key `media-user-token` and copy its value.
4. (Optional) Obtain **Apple Developer JWT**:
   - The tool auto-fetches this token automatically.
   - If you wish to set it manually, open the **Network** tab in DevTools.
   - Filter by requests to `api.music.apple.com`.
   - Inspect **Request Headers** -> `Authorization`.
   - Copy the JWT token string **without** the `Bearer ` prefix (e.g. `eyJhbGciOi...`).
5. Add tokens to `.env`:
   ```env
   APPLE_MUSIC_USER_TOKEN=your_media_user_token
   APPLE_STOREFRONT=us
   APPLE_DEVELOPER_JWT=your_developer_jwt_without_bearer
   ```

---

## 🏃 Usage

### Build the Binary
```bash
go build -o spotapp-migrator ./cmd/migrator
```

### Run Interactive Migration
```bash
./spotapp-migrator
```

### Options & Flags
- **Dry-Run (Preview only)**:
  ```bash
  ./spotapp-migrator --dry-run
  ```
- **Auto-migrate All Playlists (Non-Interactive)**:
  ```bash
  ./spotapp-migrator --yes
  ```
- **Migrate Only User-Owned Playlists**:
  ```bash
  ./spotapp-migrator --own-only
  ```
- **CSV Mode (Bypass Spotify API)**:
  ```bash
  ./spotapp-migrator --from-csv /path/to/exported_csv_folder
  ```
  *(Export your playlist CSV files first at [https://exportify.net](https://exportify.net)).*
- **Custom Config File**:
  ```bash
  ./spotapp-migrator --config custom.env
  ```
