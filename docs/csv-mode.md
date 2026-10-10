# Modo CSV (Exportify)

Spotify restringió (nov. 2024) el endpoint de tracks de playlists para apps en
**Development Mode**, devolviendo `403 Forbidden` incluso para las playlists
propias del usuario. Obtener *Extended Quota Mode* requiere una revisión de
Spotify pensada para apps públicas, por lo que se añade un modo alternativo que
**no usa la API de Spotify** para leer las canciones.

## Uso

1. Exporta tus playlists a CSV con [Exportify](https://exportify.net).
2. Coloca los `.csv` en un directorio (cada archivo = una playlist; el nombre del
   archivo se usa como nombre de la playlist).
3. Ejecuta el migrador con el flag `--from-csv`:

   ```bash
   go run ./cmd/migrator --from-csv ./export --yes
   ```

En este modo se omite toda la autenticación y las llamadas a la API de Spotify.
El resto del flujo (matching + creación de playlists en Apple Music) es idéntico.

## Formato esperado (Exportify)

Las columnas relevantes del CSV de Exportify son:

- `Track Name`
- `Artist Name(s)` (varios artistas separados por comas)
- `Album Name`
- `Track Duration (ms)`
- `ISRC`
- `Track URI`

El parser es tolerante: ignora columnas desconocidas y filas sin nombre de pista.

## Componentes implicados

- **`pkg/spotify/csv.go`**: `LoadPlaylistsFromCSV(dir)` parsea el directorio de
  CSV y devuelve `[]Playlist` + `map[playlistID][]Track`.
- **`pkg/spotify/spotify.go`**: el `Client` tiene un campo opcional `localTracks`.
  `NewCSVClient(dir)` crea un cliente que sirve los datos desde CSV, y
  `GetPlaylistTracks` devuelve las tracks cacheadas en lugar de llamar a la API.
- **`cmd/migrator/main.go`**: el flag `--from-csv` activa el modo CSV, saltando
  los pasos de credenciales, autenticación y descarga de playlists de Spotify.
