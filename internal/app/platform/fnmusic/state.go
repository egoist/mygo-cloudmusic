package fnmusic

import (
	"context"
	"errors"
	"strings"
	"sync"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/fnmusic"
)

// Feiniu application adapter.
//
// This package owns the platform session, state and views exposed to the
// shared app seam. Backend protocol details stay in internal/platform/fnmusic;
// the adapter must not import the root app package.

// State is the part of a Feiniu session that belongs to the platform. It is
// deliberately independent of windows, routes, MyGo UI elements and the
// shared player; the root app supplies those through a Host when rendering.
type State struct {
	mu sync.RWMutex

	client            *backend.Client
	provider          *backend.Provider
	config            backend.Config
	connectGen        uint64
	connecting        bool
	capabilities      backend.FolderCapabilities
	capabilitiesKnown bool
	post              func(func())
	err               error
	playlists         []netease.Playlist
	liked             map[int64]bool
	likeBusy          map[int64]bool
	roamID            string
	catalog           *Catalog
}

// NewState returns an empty disconnected platform session.
func NewState() *State {
	s := &State{liked: make(map[int64]bool), likeBusy: make(map[int64]bool)}
	s.catalog = NewCatalog(s, nil)
	return s
}

// Catalog returns the platform-owned lazy catalog state.
func (s *State) Catalog() *Catalog {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catalog == nil {
		s.catalog = NewCatalog(s, s.post)
	}
	return s.catalog
}

// SetPost supplies the owner goroutine used by catalog page completions.
func (s *State) SetPost(post func(func())) {
	if s == nil {
		return
	}
	if post == nil {
		post = func(fn func()) { fn() }
	}
	s.mu.Lock()
	s.post = post
	if s.catalog != nil {
		s.catalog.Post = post
	}
	s.mu.Unlock()
}

// Config returns the non-secret connection settings for this platform.
func (s *State) Config() backend.Config {
	if s == nil {
		return backend.Config{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// SetConfig stores non-secret connection settings without connecting.
func (s *State) SetConfig(cfg backend.Config) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.config = cfg
	s.mu.Unlock()
}

func (s *State) beginConnect() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connectGen++
	return s.connectGen
}

func (s *State) setConnecting(gen uint64, on bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.connectGen == gen {
		s.connecting = on
	}
	s.mu.Unlock()
}

func (s *State) finishConnecting() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.connecting = false
	s.mu.Unlock()
}

// Connecting reports an automatic or manual login that has not published its
// final catalog state yet. The home uses this to keep a loading surface up
// instead of flashing the connection form.
func (s *State) Connecting() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connecting
}

// Generation identifies the current connection session. UI actions capture
// it before starting asynchronous work and ignore completions after a
// reconnect, disconnect or platform switch.
func (s *State) Generation() uint64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connectGen
}

// Cancel invalidates in-flight platform work without logging out the active
// client. The owner uses it when switching away from this platform.
func (s *State) Cancel() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.connectGen++
	s.connecting = false
	s.mu.Unlock()
}

func (s *State) connectCurrent(gen uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connectGen == gen
}

// activeClient takes a single snapshot for a background operation. Keeping
// the client and generation together prevents a detach or reconnect between
// the connectivity check and the actual request.
func (s *State) activeClient(ctx context.Context) (*backend.Client, uint64, error) {
	if s == nil {
		return nil, 0, errors.New("fnmusic: nil state")
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	s.mu.RLock()
	c, gen := s.client, s.connectGen
	s.mu.RUnlock()
	if c == nil {
		return nil, gen, backend.ErrSignedOut
	}
	return c, gen, nil
}

func (s *State) clientCurrent(c *backend.Client, gen uint64) bool {
	if s == nil || c == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client == c && s.connectGen == gen
}

// Client returns the current backend client, if connected.
func (s *State) Client() *backend.Client {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

// Provider returns the shared platform adapter for the current session.
func (s *State) Provider() *backend.Provider {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.provider
}

// FetchArtwork handles only resources owned by this platform. Returning
// handled=false lets the shared app keep its normal NetEase fetch path without
// knowing platform URL schemes.
func (s *State) FetchArtwork(ctx context.Context, raw string, size int) (data []byte, handled bool, err error) {
	if s == nil || !strings.HasPrefix(raw, "fnmusic://") {
		return nil, false, nil
	}
	p := s.Provider()
	if p == nil {
		return nil, true, backend.ErrSignedOut
	}
	data, err = p.Cover(ctx, raw, size)
	return data, true, err
}

// Connected reports whether a client is attached.
func (s *State) Connected() bool { return s.Client() != nil }

// Error returns the latest connection or account error.
func (s *State) Error() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

// SetError records an account or connection error without coupling the
// platform package to the root app's toast and error presentation.
func (s *State) SetError(err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

// Attach makes client the active session. It does not connect or log in; the
// caller controls that lifecycle and can publish the result through Host.
func (s *State) Attach(client *backend.Client) error {
	if s == nil {
		return errors.New("fnmusic: nil state")
	}
	if client == nil {
		return errors.New("fnmusic: nil client")
	}
	s.mu.Lock()
	s.attachLocked(client)
	s.connecting = false
	s.mu.Unlock()
	return nil
}

// attachCurrent publishes client only when the connection attempt that made
// it is still the active one. The generation check and publication share a
// lock, so a concurrent disconnect cannot slip between them.
func (s *State) attachCurrent(client *backend.Client, gen uint64) bool {
	if s == nil || client == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connectGen != gen {
		return false
	}
	s.attachLocked(client)
	return true
}

func (s *State) attachLocked(client *backend.Client) {
	s.client = client
	s.provider = backend.Wrap(client)
	s.config = client.Config()
	s.err = nil
	s.capabilities = backend.FolderCapabilities{}
	s.capabilitiesKnown = false
	s.playlists = nil
	s.roamID = ""
	s.catalog = NewCatalog(s, s.post)
	s.liked = make(map[int64]bool)
	s.likeBusy = make(map[int64]bool)
}

// FolderViewEnabled reports the server capability after a successful
// connection. Unknown capabilities are treated as unavailable so older
// Feiniu Music versions do not expose a broken route.
func (s *State) FolderViewEnabled() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.capabilitiesKnown && s.capabilities.FolderView
}

// LoadCapabilities reads optional server capabilities for this session.
func (s *State) LoadCapabilities(ctx context.Context) (backend.FolderCapabilities, error) {
	c, gen, err := s.activeClient(ctx)
	if err != nil {
		return backend.FolderCapabilities{}, err
	}
	caps, err := c.Capabilities()
	if err != nil {
		return backend.FolderCapabilities{}, err
	}
	if !s.clientCurrent(c, gen) {
		return backend.FolderCapabilities{}, context.Canceled
	}
	s.mu.Lock()
	s.capabilities, s.capabilitiesKnown = caps, true
	s.mu.Unlock()
	return caps, nil
}

// Detach clears the active session and returns its client for logout. The
// platform state is reset together, so a later login cannot see stale lists.
func (s *State) Detach() *backend.Client {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	client := s.client
	s.connectGen++
	s.client, s.provider, s.err = nil, nil, nil
	s.capabilities, s.capabilitiesKnown = backend.FolderCapabilities{}, false
	s.connecting = false
	s.config = backend.Config{}
	s.playlists = nil
	s.roamID = ""
	s.catalog = NewCatalog(s, s.post)
	s.liked = make(map[int64]bool)
	s.likeBusy = make(map[int64]bool)
	s.mu.Unlock()
	return client
}

// Playlists returns a copy of the account playlists held by this session.
func (s *State) Playlists() []netease.Playlist {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]netease.Playlist(nil), s.playlists...)
}

// SetPlaylists replaces the account playlists with a copy owned by State.
func (s *State) SetPlaylists(lists []netease.Playlist) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.playlists = append([]netease.Playlist(nil), lists...)
	s.mu.Unlock()
}

// Playlist returns a copy of an account playlist. Copies keep callers from
// mutating session state while a request is in flight.
func (s *State) Playlist(id int64) (netease.Playlist, bool) {
	for _, pl := range s.Playlists() {
		if pl.ID == id {
			return pl, true
		}
	}
	return netease.Playlist{}, false
}

func (s *State) playlistClient(ctx context.Context) (*backend.Client, uint64, error) {
	return s.activeClient(ctx)
}

// CreatePlaylist creates a playlist and publishes it in account state.
func (s *State) CreatePlaylist(ctx context.Context, name string) (netease.Playlist, error) {
	c, gen, err := s.playlistClient(ctx)
	if err != nil {
		return netease.Playlist{}, err
	}
	pl, err := c.CreatePlaylist(name)
	if err != nil {
		return netease.Playlist{}, err
	}
	if !s.clientCurrent(c, gen) {
		return netease.Playlist{}, context.Canceled
	}
	s.mu.Lock()
	s.playlists = append([]netease.Playlist{pl}, s.playlists...)
	s.mu.Unlock()
	return pl, nil
}

func (s *State) RenamePlaylist(ctx context.Context, pl netease.Playlist, name string) error {
	c, gen, err := s.playlistClient(ctx)
	if err != nil {
		return err
	}
	if err = c.RenamePlaylist(pl.Ref, name); err != nil {
		return err
	}
	if !s.clientCurrent(c, gen) {
		return context.Canceled
	}
	s.mu.Lock()
	for i := range s.playlists {
		if s.playlists[i].ID == pl.ID {
			s.playlists[i].Name = name
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *State) DeletePlaylist(ctx context.Context, pl netease.Playlist) error {
	c, gen, err := s.playlistClient(ctx)
	if err != nil {
		return err
	}
	if err = c.DeletePlaylist(pl.Ref); err != nil {
		return err
	}
	if !s.clientCurrent(c, gen) {
		return context.Canceled
	}
	s.mu.Lock()
	for i := range s.playlists {
		if s.playlists[i].ID == pl.ID {
			s.playlists = append(s.playlists[:i], s.playlists[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *State) AddToPlaylist(ctx context.Context, pl netease.Playlist, tracks []netease.Track) error {
	c, gen, err := s.playlistClient(ctx)
	if err != nil {
		return err
	}
	refs := make([]string, 0, len(tracks))
	for _, tr := range tracks {
		if tr.Ref != "" {
			refs = append(refs, tr.Ref)
		}
	}
	if len(refs) == 0 {
		return nil
	}
	if err = c.AddToPlaylist(pl.Ref, refs); err != nil {
		return err
	}
	if !s.clientCurrent(c, gen) {
		return context.Canceled
	}
	return nil
}

func (s *State) RemoveFromPlaylist(ctx context.Context, pl netease.Playlist, tr netease.Track) error {
	c, gen, err := s.playlistClient(ctx)
	if err != nil {
		return err
	}
	if err = c.RemoveFromPlaylist(pl.Ref, []string{tr.Ref}); err != nil {
		return err
	}
	if !s.clientCurrent(c, gen) {
		return context.Canceled
	}
	return nil
}

func (s *State) ChangePlaylistCover(ctx context.Context, pl netease.Playlist, picture []byte) (string, error) {
	c, gen, err := s.playlistClient(ctx)
	if err != nil {
		return "", err
	}
	id, err := c.SetPlaylistCover(pl.Ref, pl.Name, picture)
	if err != nil {
		return "", err
	}
	if !s.clientCurrent(c, gen) {
		return "", context.Canceled
	}
	lists := s.Playlists()
	for i := range lists {
		if lists[i].ID == pl.ID {
			lists[i].CoverURL = backend.CoverURL(id)
		}
	}
	s.SetPlaylists(lists)
	return id, nil
}

// Liked reports the optimistic liked state of a track.
func (s *State) Liked(id int64) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.liked[id]
}

// LikedTrack reports the optimistic liked state for a provider track.
func (s *State) LikedTrack(tr *netease.Track) bool {
	if tr == nil {
		return false
	}
	return s.Liked(tr.ID)
}

// SetLiked updates the optimistic liked state.
func (s *State) SetLiked(id int64, liked bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if liked {
		s.liked[id] = true
	} else {
		delete(s.liked, id)
	}
	s.mu.Unlock()
}

// LikeBusy reports whether a like operation is in flight for id.
func (s *State) LikeBusy(id int64) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.likeBusy[id]
}

// SetLikeBusy marks a like operation as in flight or complete.
func (s *State) SetLikeBusy(id int64, busy bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if busy {
		s.likeBusy[id] = true
	} else {
		delete(s.likeBusy, id)
	}
	s.mu.Unlock()
}

// SetRoamID and RoamID keep the service's roaming cursor in platform state.
func (s *State) SetRoamID(id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.roamID = id
	s.mu.Unlock()
}

func (s *State) RoamID() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.roamID
}

// CheckContext lets asynchronous platform jobs reject stale or canceled work
// before invoking a backend operation.
func (s *State) CheckContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.Connected() {
		return backend.ErrSignedOut
	}
	return nil
}
