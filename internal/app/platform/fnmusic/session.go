package fnmusic

import (
	"context"
	"errors"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/fnmusic"
)

// Connect creates and signs in a new Feiniu client, then publishes it as the
// active session. The caller owns configuration persistence and presentation
// of errors; this package only owns platform lifecycle.
func (s *State) Connect(ctx context.Context, cfg backend.Config) error {
	if s == nil {
		return errors.New("fnmusic: nil state")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	gen := s.beginConnect()
	s.setConnecting(gen, true)
	defer s.setConnecting(gen, false)
	c, err := backend.New(cfg)
	if err != nil {
		if s.connectCurrent(gen) {
			s.SetError(err)
		}
		return err
	}
	if err := c.Connect(); err != nil {
		if s.connectCurrent(gen) {
			s.SetError(err)
		}
		return err
	}
	if !s.connectCurrent(gen) {
		c.Logout()
		return context.Canceled
	}
	if err := ctx.Err(); err != nil {
		c.Logout()
		return err
	}
	if err := c.Login(); err != nil {
		if s.connectCurrent(gen) {
			s.SetError(err)
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		c.Logout()
		return err
	}
	if !s.connectCurrent(gen) {
		c.Logout()
		return context.Canceled
	}
	if !s.attachCurrent(c, gen) {
		c.Logout()
		return context.Canceled
	}
	// Capability discovery is optional for older servers. A failed probe keeps
	// the account connected and leaves optional platform pages hidden. Preserve
	// cancellation so callers can stop a connect cleanly.
	if _, err := s.LoadCapabilities(ctx); errors.Is(err, context.Canceled) {
		if old := s.Detach(); old != nil {
			old.Logout()
		}
		return err
	}
	return nil
}

// Close logs out and detaches the current session. It is safe to call when
// the platform is already disconnected.
func (s *State) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c := s.Detach()
	if c == nil {
		return nil
	}
	c.Logout()
	return nil
}

// LoadPlaylists reads the account playlists through the platform provider and
// stores a private copy in the session.
func (s *State) LoadPlaylists(ctx context.Context) ([]netease.Playlist, error) {
	c, gen, err := s.activeClient(ctx)
	if err != nil {
		return nil, err
	}
	lists, err := c.Playlists()
	if err != nil {
		if s.clientCurrent(c, gen) {
			s.SetError(err)
		}
		return nil, err
	}
	if !s.clientCurrent(c, gen) {
		return nil, context.Canceled
	}
	s.SetPlaylists(lists)
	s.SetError(nil)
	return s.Playlists(), nil
}

// SetFavorite updates the optimistic state and persists the change through
// the platform backend. A failed request rolls the optimistic state back.
func (s *State) SetFavorite(ctx context.Context, tr *netease.Track, liked bool) error {
	if tr == nil {
		return errors.New("fnmusic: nil track")
	}
	c, gen, err := s.activeClient(ctx)
	if err != nil {
		return err
	}
	old := s.Liked(tr.ID)
	s.SetLiked(tr.ID, liked)
	s.SetLikeBusy(tr.ID, true)
	err = c.SetFavorite(tr.Ref, liked)
	if !s.clientCurrent(c, gen) {
		s.SetLikeBusy(tr.ID, false)
		return context.Canceled
	}
	s.SetLikeBusy(tr.ID, false)
	if err != nil {
		s.SetLiked(tr.ID, old)
	}
	return err
}

// ReportPlay records a started track in the service's play history.
func (s *State) ReportPlay(ctx context.Context, tr *netease.Track) error {
	if s == nil || tr == nil || tr.Ref == "" {
		return nil
	}
	c, gen, err := s.activeClient(ctx)
	if err != nil {
		return err
	}
	if err = c.ReportPlay(tr.Ref); err != nil {
		return err
	}
	if !s.clientCurrent(c, gen) {
		return context.Canceled
	}
	return nil
}

// RemoveFromHistory removes one entry from the server play history and keeps
// the operation tied to the session that issued it.
func (s *State) RemoveFromHistory(ctx context.Context, tr netease.Track) error {
	if s == nil {
		return errors.New("fnmusic: nil state")
	}
	c, gen, err := s.activeClient(ctx)
	if err != nil {
		return err
	}
	if err = c.RemoveFromHistory([]string{tr.Ref}); err != nil {
		return err
	}
	if !s.clientCurrent(c, gen) {
		return context.Canceled
	}
	return nil
}

// StartRoam asks the service for the first roaming track and records its
// cursor in the platform session.
func (s *State) StartRoam(ctx context.Context) (netease.Track, error) {
	c, gen, err := s.activeClient(ctx)
	if err != nil {
		return netease.Track{}, err
	}
	tr, id, err := c.RoamStart()
	if err != nil {
		if s.clientCurrent(c, gen) {
			s.SetError(err)
		}
		return netease.Track{}, err
	}
	if !s.clientCurrent(c, gen) {
		return netease.Track{}, context.Canceled
	}
	s.SetRoamID(id)
	s.SetError(nil)
	return tr, nil
}

// NextRoam advances the service roaming cursor.
func (s *State) NextRoam(ctx context.Context) (netease.Track, error) {
	c, gen, err := s.activeClient(ctx)
	if err != nil {
		return netease.Track{}, err
	}
	tr, id, err := c.RoamNext(s.RoamID())
	if err != nil {
		if s.clientCurrent(c, gen) {
			s.SetError(err)
		}
		return netease.Track{}, err
	}
	if !s.clientCurrent(c, gen) {
		return netease.Track{}, context.Canceled
	}
	s.SetRoamID(id)
	s.SetError(nil)
	return tr, nil
}
