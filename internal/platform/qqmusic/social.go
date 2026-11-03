package qqmusic

import (
	"encoding/json"
	"errors"
	"hash/fnv"
	"html"
	"strconv"
	"sync"
)

func PersonalFM(mode string) ([]Track, error) {
	if mode != FMDefault {
		return nil, ErrUnsupported
	}
	if sessionSnapshot().MusicKey == "" {
		return nil, errors.New("请先登录 QQ 音乐")
	}
	var data object
	if err := rpc("music.radioProxy.MbTrackRadioSvr", "get_radio_track", map[string]any{"id": 99, "firstplay": 1, "num": 15}, &data); err != nil {
		return nil, err
	}
	return convertTracks(objects(data, "tracks")), nil
}

type commentCursor struct {
	sequence    string
	page, total int
	version     json.RawMessage
}

// QQ requires the preceding page's sequence and total version, not just PageNum.
var commentCursors = struct {
	sync.Mutex
	values map[string]map[int]commentCursor
}{values: map[string]map[int]commentCursor{}}

type commentResponse struct {
	TotalVer    json.RawMessage
	CommentList *struct {
		Comments []object
		Total    int
		HasMore  json.RawMessage
	}
}

func ThreadComments(thread string, offset, limit int) (*Comments, error) {
	if _, err := strconv.ParseInt(thread, 10, 64); err != nil {
		return nil, err
	}
	// The web service rejects the old view's 30-item pages with code 10000.
	limit = min(max(limit, 1), 25)
	cursor := commentCursor{}
	if offset > 0 {
		commentCursors.Lock()
		cursor = commentCursors.values[thread][offset]
		commentCursors.Unlock()
		if cursor.sequence == "" {
			return nil, errors.New("QQ 评论分页游标已失效，请重新打开评论")
		}
	}
	data, err := readCommentPage(thread, limit, cursor, false)
	if err != nil {
		return nil, err
	}
	latest, err := decodeComments(data)
	if err != nil {
		return nil, err
	}
	result := &Comments{Total: data.CommentList.Total, Latest: latest}
	if offset == 0 {
		hot, err := readCommentPage(thread, limit, commentCursor{}, true)
		if err != nil {
			return nil, err
		}
		result.Hot, err = decodeComments(hot)
		if err != nil {
			return nil, err
		}
	}
	entries := data.CommentList.Comments
	if len(entries) > 0 {
		next := text(entries[len(entries)-1], "SeqNo")
		more := string(data.CommentList.HasMore)
		result.More = next != "" && (more == "1" || more == "true")
		if result.More {
			commentCursors.Lock()
			if offset == 0 || commentCursors.values[thread] == nil {
				commentCursors.values[thread] = map[int]commentCursor{}
			}
			commentCursors.values[thread][offset+len(latest)] = commentCursor{next, cursor.page + 1, result.Total, data.TotalVer}
			commentCursors.Unlock()
		}
	}
	return result, nil
}

func readCommentPage(thread string, limit int, cursor commentCursor, hot bool) (*commentResponse, error) {
	param := map[string]any{"BizType": 1, "BizId": thread, "PageSize": limit, "PageNum": cursor.page, "LastCommentSeqNo": cursor.sequence, "PicEnable": 1}
	method := "GetNewCommentList"
	if hot {
		method = "GetHotCommentList"
		param["HotType"], param["WithAirborne"] = 1, 0
	} else {
		param["FromCommentId"], param["WithHot"], param["LastTotal"] = "", 0, cursor.total
		param["LastTotalVer"] = "0"
		if len(cursor.version) > 0 {
			param["LastTotalVer"] = cursor.version
		}
	}
	var data commentResponse
	comm := map[string]any{"ct": 24, "cv": 0}
	if err := rpcWithComm("music.globalComment.CommentRead", method, param, &data, comm, map[string]string{"Cookie": ""}); err != nil {
		return nil, err
	}
	return &data, nil
}

func decodeComments(data *commentResponse) ([]Comment, error) {
	if data.CommentList == nil || data.CommentList.Comments == nil {
		return nil, errors.New("QQ 音乐未返回评论列表")
	}
	entries := data.CommentList.Comments
	out := make([]Comment, 0, len(entries))
	for _, entry := range entries {
		remote := text(entry, "CmId")
		if remote == "" {
			continue
		}
		id := number(entry, "CmId")
		if id == 0 {
			h := fnv.New64a()
			_, _ = h.Write([]byte(remote))
			id = int64(h.Sum64() & 0x7fffffffffffffff)
		}
		c := Comment{ID: id, RemoteID: remote, Content: html.UnescapeString(text(entry, "Content")), Time: number(entry, "PubTime") * 1000, LikedCount: int(number(entry, "PraiseNum"))}
		c.User.Nickname, c.User.AvatarURL = text(entry, "Nick"), text(entry, "Avatar")
		out = append(out, c)
	}
	if len(entries) > 0 && len(out) == 0 {
		return nil, errors.New("QQ 音乐未返回有效评论")
	}
	return out, nil
}

func UserDetail(id int64) (*User, error) {
	p, err := Account()
	if err != nil {
		return nil, err
	}
	if p == nil || p.UserID != id {
		return nil, ErrUnsupported
	}
	return &User{UserID: p.UserID, Nickname: p.Nickname, AvatarURL: p.AvatarURL}, nil
}
