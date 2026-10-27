package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"netease-music-mygo/internal/netease"
)

// The count on the comments' tab holds the last song's while the next
// one's loads, for it to roll from one to the other, and goes once the
// next song turns out to have none.
func TestCommentCountHoldsWhileLoading(t *testing.T) {
	a, tt, _ := coverApp(t)
	a.comments = commentState{track: 2, loaded: true, total: 128}
	tt.Frame()
	a.pl.Next()
	a.comments = commentState{track: 3, loading: true}
	tt.Frame()
	if _, ok := tt.Find("评论 128"); !ok {
		t.Fatal("the count went as the next song's loads")
	}
	a.comments.loading, a.comments.loaded, a.comments.total = false, true, 96
	tt.Frame()
	if _, ok := tt.Find("评论 96"); !ok {
		t.Error("the count did not change once loaded")
	}
	a.pl.Next()
	a.comments = commentState{track: 1, loaded: true}
	tt.Advance(time.Second)
	tt.Frame()
	if _, ok := tt.Find("评论"); !ok {
		t.Error("a song without comments shows a count")
	}
}

// Now Playing shows an episode's comments, the episode's and not its
// audio's, under a tab of their own beside what it is about, which stays
// chosen as the frames go.
func TestEpisodeComments(t *testing.T) {
	asked := make(chan string, 4)
	threadComments = func(thread string, offset, limit int) (*netease.Comments, error) {
		asked <- thread
		return &netease.Comments{}, nil
	}
	defer func() { threadComments = netease.ThreadComments }()
	a := newTestApp()
	a.settings = defaultSettings()
	a.pl.Cue([]netease.Track{{ID: 13, Name: "第一期", ProgramID: 3, PodcastID: 5, Duration: 600000}})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	if thread := wait(t, asked, "the comments"); thread != "A_DJ_1_3" {
		t.Fatalf("asked for the comments of %s", thread)
	}
	if a.npPane != paneProgram {
		t.Fatalf("opened on pane %d", a.npPane)
	}
	if err := tt.Click("评论"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if a.npPane != paneComments {
		t.Errorf("on pane %d", a.npPane)
	}
}

// 赞 on a comment likes it at once, counting the listener in where it is
// hot and where it is latest, and asks NetEase to; again, it unlikes it.
func TestLikeComment(t *testing.T) {
	type call struct {
		thread string
		id     int64
		like   bool
	}
	calls := make(chan call, 4)
	likeComment = func(thread string, id int64, like bool) error { calls <- call{thread, id, like}; return nil }
	defer func() { likeComment = netease.LikeComment }()
	a, tt, _ := coverApp(t)
	a.profile = &netease.Profile{UserID: 1}
	cm := netease.Comment{ID: 9, Content: "好听", LikedCount: 41}
	a.comments = commentState{track: 2, thread: "R_SO_4_2", loaded: true, total: 1, hot: []netease.Comment{cm}, latest: []netease.Comment{cm}}
	a.npPane = paneComments
	tt.Frame()
	if err := tt.Click("赞"); err != nil {
		t.Fatal(err)
	}
	if c := wait(t, calls, "the like"); c != (call{"R_SO_4_2", 9, true}) {
		t.Errorf("asked NetEase for %+v", c)
	}
	for _, cm := range []netease.Comment{a.comments.hot[0], a.comments.latest[0]} {
		if !cm.Liked || cm.LikedCount != 42 {
			t.Fatalf("liked %v by %d", cm.Liked, cm.LikedCount)
		}
	}
	tt.Frame()
	if err := tt.Click("取消赞"); err != nil {
		t.Fatal(err)
	}
	if c := wait(t, calls, "the unlike"); c.like {
		t.Error("liked again")
	}
	for _, cm := range []netease.Comment{a.comments.hot[0], a.comments.latest[0]} {
		if cm.Liked || cm.LikedCount != 41 {
			t.Errorf("unliked: liked %v by %d", cm.Liked, cm.LikedCount)
		}
	}
}
