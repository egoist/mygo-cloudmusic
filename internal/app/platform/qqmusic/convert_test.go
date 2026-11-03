package qqmusic

import (
	"testing"

	"netease-music-mygo/internal/netease"
	backend "netease-music-mygo/internal/platform/qqmusic"
)

// QQ threads are the song ids themselves, not NetEase's R_SO_4_ prefix.
func TestCommentThreadUsesQQTrackID(t *testing.T) {
	a := &Adapter{}
	if got := a.CommentThread(&netease.Track{ID: 42}); got != "42" {
		t.Fatalf("thread = %q, want 42", got)
	}
	if got := a.CommentThread(nil); got != "" {
		t.Fatalf("nil track thread = %q", got)
	}
}

// QQ's comment shape carries over to the shared comments pane.
func TestQQCommentsConvert(t *testing.T) {
	in := &backend.Comments{
		Total: 7,
		More:  true,
		Hot: []backend.Comment{{
			ID: 3, Content: "好听", Time: 1700000000000, LikedCount: 9,
		}},
		Latest: []backend.Comment{{
			ID: 4, Content: "回复", Time: 1700000001000,
		}},
	}
	in.Hot[0].User.Nickname, in.Hot[0].User.AvatarURL = "甲", "https://img/1"
	in.Hot[0].IPLocation.Location = "广东"
	in.Latest[0].BeReplied = append(in.Latest[0].BeReplied, struct {
		User    struct{ Nickname string }
		Content string
	}{})
	in.Latest[0].BeReplied[0].User.Nickname, in.Latest[0].BeReplied[0].Content = "乙", "原评论"

	out := qqComments(in)
	if out.Total != 7 || !out.More || len(out.Hot) != 1 || len(out.Latest) != 1 {
		t.Fatalf("converted = %#v", out)
	}
	if out.Hot[0].User.Nickname != "甲" || out.Hot[0].IPLocation.Location != "广东" || out.Hot[0].LikedCount != 9 {
		t.Fatalf("hot comment = %#v", out.Hot[0])
	}
	if len(out.Latest[0].BeReplied) != 1 || out.Latest[0].BeReplied[0].User.Nickname != "乙" {
		t.Fatalf("reply = %#v", out.Latest[0].BeReplied)
	}
}

// QQ sends chart and album prose as HTML: breaks become lines, tags go.
func TestCleanDescription(t *testing.T) {
	in := "1.定义：热度飙升最快的前100首歌曲。<br>2.更新频率：每日更新。<br />3.排序：<b>日环比</b>&amp;播放"
	want := "1.定义：热度飙升最快的前100首歌曲。\n2.更新频率：每日更新。\n3.排序：日环比&播放"
	if got := cleanDescription(in); got != want {
		t.Fatalf("cleanDescription = %q, want %q", got, want)
	}
}
