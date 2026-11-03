package qqmusic

import (
	"context"
	"sync"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/qqmusic"
)

// State owns the part of the QQ Music session shared by the adapter and its
// pages: the account, the playlists, and the optimistic liked set.
type State struct {
	mu        sync.RWMutex
	profile   *backend.Profile
	playlists []netease.Playlist
	liked     map[int64]bool
	err       error
}

func NewState() *State {
	return &State{liked: map[int64]bool{}}
}

func (s *State) Profile() *backend.Profile {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.profile
}

func (s *State) SetProfile(p *backend.Profile) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.profile = p
	s.mu.Unlock()
}

func (s *State) Playlists() []netease.Playlist {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]netease.Playlist(nil), s.playlists...)
}

func (s *State) SetPlaylists(lists []netease.Playlist) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.playlists = append([]netease.Playlist(nil), lists...)
	s.mu.Unlock()
}

func (s *State) FindPlaylist(id int64) (netease.Playlist, bool) {
	if s == nil {
		return netease.Playlist{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, pl := range s.playlists {
		if pl.ID == id {
			return pl, true
		}
	}
	return netease.Playlist{}, false
}

func (s *State) Liked(id int64) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.liked[id]
}

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

func (s *State) SetLikedIDs(ids []int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.liked = make(map[int64]bool, len(ids))
	for _, id := range ids {
		s.liked[id] = true
	}
	s.mu.Unlock()
}

func (s *State) Error() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func (s *State) SetError(err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

func (s *State) SetFavorite(ctx context.Context, tr *netease.Track, liked bool) error {
	if s == nil || tr == nil {
		return context.Canceled
	}
	old := s.Liked(tr.ID)
	s.SetLiked(tr.ID, liked)
	if err := backend.Like(tr.ID, liked); err != nil {
		s.SetLiked(tr.ID, old)
		return err
	}
	return nil
}
