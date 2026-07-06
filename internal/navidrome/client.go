package navidrome

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrMutationRejected marks server failures while changing playlist state.
var ErrMutationRejected = errors.New("navidrome mutation rejected")

// APIError is a Subsonic/OpenSubsonic error response.
type APIError struct {
	Code    int
	Message string
}

func (e APIError) Error() string {
	return fmt.Sprintf("navidrome API error %d: %s", e.Code, e.Message)
}

// Client calls Navidrome's Subsonic/OpenSubsonic API.
type Client struct {
	cfg        Config
	httpClient *http.Client
	salt       func() string
}

// NewClient returns a Navidrome API client.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		salt:       randomSalt,
	}
}

// Searcher is the API surface needed by the resolver.
type Searcher interface {
	SearchSongs(ctx context.Context, query string) ([]Song, error)
	GetSong(ctx context.Context, id string) (Song, error)
}

type subsonicResponse struct {
	Subsonic responseStatus `json:"subsonic-response"`
}

type responseStatus struct {
	Status   string            `json:"status"`
	Error    *subsonicError    `json:"error,omitempty"`
	Search   *searchResponse   `json:"searchResult3,omitempty"`
	Playlist *playlistPayload  `json:"playlist,omitempty"`
	List     *playlistsPayload `json:"playlists,omitempty"`
	Song     *Song             `json:"song,omitempty"`
}

type subsonicError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type searchResponse struct {
	Songs []Song `json:"song"`
}

type playlistsPayload struct {
	Playlists []Playlist `json:"playlist"`
}

type playlistPayload struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Owner   string `json:"owner"`
	Public  bool   `json:"public"`
	Comment string `json:"comment"`
	Songs   []Song `json:"entry"`
}

// Ping validates authentication and server reachability.
func (c *Client) Ping(ctx context.Context) error {
	var out subsonicResponse
	return c.get(ctx, "ping.view", nil, &out)
}

// SearchSongs searches songs through search3.view.
func (c *Client) SearchSongs(ctx context.Context, query string) ([]Song, error) {
	form := url.Values{
		"query":       {query},
		"songCount":   {"20"},
		"albumCount":  {"0"},
		"artistCount": {"0"},
	}
	var out subsonicResponse
	if err := c.get(ctx, "search3.view", form, &out); err != nil {
		return nil, err
	}
	if out.Subsonic.Search == nil {
		return nil, nil
	}
	return out.Subsonic.Search.Songs, nil
}

// GetSong verifies that a pinned song id exists.
func (c *Client) GetSong(ctx context.Context, id string) (Song, error) {
	form := url.Values{"id": {id}}
	var out subsonicResponse
	if err := c.get(ctx, "getSong.view", form, &out); err != nil {
		return Song{}, err
	}
	if out.Subsonic.Song == nil {
		return Song{}, ErrNotFound
	}
	return *out.Subsonic.Song, nil
}

// GetPlaylists returns server playlists.
func (c *Client) GetPlaylists(ctx context.Context) ([]Playlist, error) {
	var out subsonicResponse
	if err := c.get(ctx, "getPlaylists.view", nil, &out); err != nil {
		return nil, err
	}
	if out.Subsonic.List == nil {
		return nil, nil
	}
	return out.Subsonic.List.Playlists, nil
}

// GetPlaylist returns one playlist with entries.
func (c *Client) GetPlaylist(ctx context.Context, id string) (Playlist, []Song, error) {
	form := url.Values{"id": {id}}
	var out subsonicResponse
	if err := c.get(ctx, "getPlaylist.view", form, &out); err != nil {
		return Playlist{}, nil, err
	}
	if out.Subsonic.Playlist == nil {
		return Playlist{}, nil, ErrNotFound
	}
	payload := out.Subsonic.Playlist
	return Playlist{
		ID:      payload.ID,
		Name:    payload.Name,
		Owner:   payload.Owner,
		Public:  payload.Public,
		Comment: payload.Comment,
	}, payload.Songs, nil
}

// CreateOrReplacePlaylist creates a playlist or replaces all entries when id is set.
func (c *Client) CreateOrReplacePlaylist(ctx context.Context, playlistID, name string, songIDs []string) error {
	form := url.Values{"name": {name}}
	if playlistID != "" {
		form.Set("playlistId", playlistID)
	}
	for _, id := range songIDs {
		form.Add("songId", id)
	}
	var out subsonicResponse
	return c.post(ctx, "createPlaylist.view", form, &out)
}

// UpdatePlaylistMetadata updates comment/public metadata after contents apply.
func (c *Client) UpdatePlaylistMetadata(ctx context.Context, playlistID string, public *bool, comment *string) error {
	form := url.Values{"playlistId": {playlistID}}
	if public != nil {
		form.Set("public", fmt.Sprintf("%t", *public))
	}
	if comment != nil {
		form.Set("comment", *comment)
	}
	var out subsonicResponse
	return c.post(ctx, "updatePlaylist.view", form, &out)
}

func (c *Client) get(ctx context.Context, endpoint string, form url.Values, out *subsonicResponse) error {
	if form == nil {
		form = url.Values{}
	}
	applyAuth(form, c.cfg, c.salt())
	reqURL := c.cfg.Server + "/rest/" + endpoint + "?" + form.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, endpoint string, form url.Values, out *subsonicResponse) error {
	applyAuth(form, c.cfg, c.salt())
	reqURL := c.cfg.Server + "/rest/" + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out *subsonicResponse) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("navidrome request failed: status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode Navidrome response: %w", err)
	}
	if out.Subsonic.Status != "" && out.Subsonic.Status != "ok" {
		if out.Subsonic.Error != nil {
			return APIError{Code: out.Subsonic.Error.Code, Message: out.Subsonic.Error.Message}
		}
		return fmt.Errorf("navidrome API status: %s", out.Subsonic.Status)
	}
	return nil
}

func applyAuth(values url.Values, cfg Config, salt string) {
	sum := md5.Sum([]byte(cfg.Password + salt))
	values.Set("u", cfg.Username)
	values.Set("t", hex.EncodeToString(sum[:]))
	values.Set("s", salt)
	values.Set("v", cfg.APIVersion)
	values.Set("c", cfg.Client)
	values.Set("f", "json")
}

func randomSalt() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
