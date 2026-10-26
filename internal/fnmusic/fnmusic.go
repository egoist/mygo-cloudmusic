// Package fnmusic is the music service of fnOS, Feiniu's NAS: the songs
// the listener keeps on their own server, as the web player of that
// service reads them.
package fnmusic

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"netease-music-mygo/internal/netease"
)

// Source is what tracks of this service are named as, in their Source.
const Source = "fn"

// Config is how to reach the service, and who is listening: the
// password's hash is all that signing in takes, so it is all that is kept.
type Config struct {
	// Mode is how Address says where the service is: ModeAddress, or
	// ModeFNConnect.
	Mode string `json:"mode,omitempty"`
	// Address is where the service is: a host, host:port, or a URL; or, in
	// ModeFNConnect, the FN ID of the NAS.
	Address string `json:"address"`
	// HTTPS says the address is reached over HTTPS, which the listener
	// chooses: an address with no scheme is taken for what this says, and
	// one with a scheme for its own.
	HTTPS    bool   `json:"https,omitempty"`
	Username string `json:"username"`
	// DeviceID names this app to the service, which knows one session of
	// each.
	DeviceID string `json:"deviceID"`
	// PassHash and AccessCode are secrets, which the app keeps in the
	// system's credential store, not with the rest.
	PassHash string `json:"-"`
	// AccessCode is the code a NAS with an access code asks for, or "".
	AccessCode string `json:"-"`
}

// How an address says where the service is.
const (
	ModeAddress   = ""
	ModeFNConnect = "fnconnect"
)

// Valid reports whether the config has all it takes to sign in.
func (c Config) Valid() bool {
	if c.Username == "" || c.PassHash == "" {
		return false
	}
	if c.Mode == ModeFNConnect {
		_, ok := FNID(c.Address)
		return ok
	}
	return strings.TrimSpace(c.Address) != ""
}

// HashPassword is what the service takes of a password.
func HashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

// NewDeviceID makes a name for this app, to keep.
func NewDeviceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// The service's web player signs each request with these, which its
// server checks.
const (
	signPrefix = "NDzZTVxnRKP8Z0jXg1VAMonaG8akvh"
	signKey    = "6D5602D4-A342-4799-A0F0-BB795E7167D0"
	apiPath    = "/music/api/v1"
)

// ErrSignedOut is what a request that needs a signed in listener returns
// without one.
var ErrSignedOut = errors.New("fnmusic: not signed in")

// errSession says the service dropped the session: its token ran out, or
// another device took its place.
var errSession = errors.New("fnmusic: session ended")

// Client talks to one service as one listener. It signs in when asked
// to, and again when its session ends.
type Client struct {
	cfg  Config
	http *http.Client

	epMu   sync.Mutex // guards ep, briefly
	ep     *endpoint
	findMu sync.Mutex // held while finding the service

	mu    sync.Mutex // guards token; held while signing in
	token string
}

// New returns a client of the service cfg names, not yet signed in. With
// an FN ID, it finds the NAS when first asked for something.
func New(cfg Config) (*Client, error) {
	c := &Client{cfg: cfg, http: newHTTPClient()}
	if cfg.Mode == ModeFNConnect {
		id, ok := FNID(cfg.Address)
		if !ok {
			return nil, ErrInvalidID
		}
		c.cfg.Address = id
	} else {
		base, err := parseAddress(cfg.Address, cfg.HTTPS)
		if err != nil {
			return nil, err
		}
		c.ep = &endpoint{base: base}
	}
	if c.cfg.DeviceID == "" {
		c.cfg.DeviceID = NewDeviceID()
	}
	return c, nil
}

// Config returns the config the client was made with.
func (c *Client) Config() Config { return c.cfg }

// parseAddress reads where the service is, from what a listener types. An
// address with no scheme takes the one the listener chose: HTTP, or HTTPS
// if https is set.
func parseAddress(addr string, https bool) (*url.URL, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, errors.New("fnmusic: no address")
	}
	if !strings.Contains(addr, "://") {
		scheme := "http://"
		if https {
			scheme = "https://"
		}
		addr = scheme + addr
	}
	u, err := url.Parse(addr)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("fnmusic: bad address %q", addr)
	}
	// The service is under /music/api/v1, however far down a reverse
	// proxy mounts it.
	p := "/" + strings.Trim(u.Path, "/")
	switch {
	case p == "/":
		p = apiPath
	case strings.HasSuffix(p, apiPath):
	case strings.HasSuffix(p, "/music"):
		p += "/api/v1"
	default:
		p += apiPath
	}
	return &url.URL{Scheme: u.Scheme, Host: u.Host, Path: p}, nil
}

// signature is the authx header of a request: its path, the query as it
// is read (or, for a request with a body, the body), a nonce and the time.
func signature(path, payload, nonce string, ms int64) string {
	return signWith(signKey, path, payload, nonce, ms)
}

// signWith is signature with a key of its own: FN Connect's lookup has one.
func signWith(key, path, payload, nonce string, ms int64) string {
	sum := md5.Sum([]byte(payload))
	ts := strconv.FormatInt(ms, 10)
	src := strings.Join([]string{signPrefix, path, nonce, ts, hex.EncodeToString(sum[:]), key}, "_")
	sign := md5.Sum([]byte(src))
	return fmt.Sprintf("nonce=%s&timestamp=%s&sign=%s", nonce, ts, hex.EncodeToString(sign[:]))
}

// readQuery is a query as the server reads it to sign: sorted, decoded.
func readQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var parts []string
	for _, k := range keys {
		for _, v := range q[k] {
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, "&")
}

// request builds a signed request of the service's API.
func (c *Client) request(method, path string, q url.Values, body []byte, token string) (*http.Request, error) {
	return c.requestWith(method, path, q, body, "application/json", string(body), token)
}

// requestWith is request for a body that is not JSON: its content type, and
// what the signature is of, which for a form is not the body. The web player
// signs what JSON.stringify makes of its FormData, which is {}.
func (c *Client) requestWith(method, path string, q url.Values, body []byte, contentType, signedBody, token string) (*http.Request, error) {
	ep, err := c.endpoint()
	if err != nil {
		return nil, err
	}
	u := *ep.base
	full := u.Path + "/" + strings.TrimLeft(path, "/")
	u.Path = full
	u.RawQuery = q.Encode()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, u.String(), r)
	if err != nil {
		return nil, err
	}
	// The server checks the path from /music/api/v1 on.
	signed := full[strings.LastIndex(full, apiPath):]
	payload := readQuery(q)
	if method != http.MethodGet {
		payload = signedBody
	}
	nonce := fmt.Sprintf("%06d", 100000+randomN(900000))
	req.Header.Set("authx", signature(signed, payload, nonce, time.Now().UnixMilli()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "zh-CN")
	req.Header.Set("User-Agent", netease.UserAgent)
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	var cookies []string
	if token != "" {
		// Escaped as a browser does: a space is a plus.
		cookies = append(cookies, "music-token="+url.QueryEscape(token))
	}
	if ep.relay {
		cookies = append(cookies, "mode=relay")
	}
	if len(cookies) > 0 {
		req.Header.Set("Cookie", strings.Join(cookies, "; "))
	}
	c.accessHeaders(req.Header)
	return req, nil
}

// accessHeaders adds the access code of an FN Connect NAS, if there is one.
func (c *Client) accessHeaders(h http.Header) {
	if c.cfg.AccessCode != "" {
		h.Set("x-access-code", base64.StdEncoding.EncodeToString([]byte(c.cfg.AccessCode)))
		h.Set("x-access-source", "app")
	}
}

// send makes a request of the service and sends it: through build, which
// makes it anew for a second go. An FN Connect NAS moves (a phone's
// network changes, the route that answered is gone), so a request that
// could not get through finds the NAS again, once.
func (c *Client) send(build func() (*http.Request, error)) (*http.Response, error) {
	return c.sendWith(build, true)
}

// sendWith is send, which asks again over another route only if replay: a
// request that makes something (a playlist) is not asked twice, as the
// answer that did not come may have been to the first.
func (c *Client) sendWith(build func() (*http.Request, error), replay bool) (*http.Response, error) {
	for retry := false; ; retry = true {
		req, err := build()
		if err != nil {
			return nil, err
		}
		res, err := c.http.Do(req)
		if err != nil && replay && !retry && c.cfg.Mode == ModeFNConnect && routeFailure(err) {
			c.Invalidate()
			continue
		}
		return res, err
	}
}

func randomN(n int) int {
	var b [4]byte
	rand.Read(b[:])
	return int((uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])) % uint32(n))
}

// envelope is what every reply of the service is wrapped in.
type envelope struct {
	Code json.Number     `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// decode reads a reply: its data into out, or the error it says. Codes
// 99999 and 120001 are an ended session.
func decode(res *http.Response, out any) error {
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return errSession
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("fnmusic: %s", res.Status)
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("fnmusic: not the music service: %w", err)
	}
	code, _ := env.Code.Int64()
	switch code {
	case 0, 200:
	case 99999, 120001, 401, 403:
		return errSession
	default:
		if env.Msg == "" {
			env.Msg = "code " + strconv.FormatInt(code, 10)
		}
		return errors.New("fnmusic: " + env.Msg)
	}
	if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// Login signs in, or fails with what the service says: ErrBadLogin for
// a wrong name or password.
func (c *Client) Login() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.login()
}

// ErrBadLogin is a refusal of the name or password.
var ErrBadLogin = errors.New("用户名或密码错误")

func (c *Client) login() error {
	body, _ := json.Marshal(map[string]string{
		"username": c.cfg.Username, "password": c.cfg.PassHash, "deviceId": c.cfg.DeviceID,
	})
	res, err := c.send(func() (*http.Request, error) {
		return c.request("POST", "/user/password-login", nil, body, "")
	})
	if err != nil {
		return err
	}
	var out struct {
		UserToken string `json:"userToken"`
	}
	if err := decode(res, &out); err != nil {
		if err == errSession {
			return ErrBadLogin
		}
		return err
	}
	if strings.TrimSpace(out.UserToken) == "" {
		return errors.New("fnmusic: the service gave no token")
	}
	c.token = strings.TrimSpace(out.UserToken)
	return nil
}

// session returns the token to sign requests with, signing in first
// without one.
func (c *Client) session() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == "" {
		if err := c.login(); err != nil {
			return "", err
		}
	}
	return c.token, nil
}

// refresh signs in again, if the session of token is still the one in use:
// another request may have done it already.
func (c *Client) refresh(token string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != token {
		return nil
	}
	c.token = ""
	return c.login()
}

// Logout ends the session, as far as the service is told.
func (c *Client) Logout() {
	c.mu.Lock()
	token := c.token
	c.token = ""
	c.mu.Unlock()
	if token == "" {
		return
	}
	res, err := c.send(func() (*http.Request, error) {
		return c.request("POST", "/user/logout", nil, nil, token)
	})
	if err == nil {
		res.Body.Close()
	}
}

// call asks the service, and reads the data of its reply into out. A
// session that ended is signed in again, once.
func (c *Client) call(method, path string, q url.Values, payload any, out any) error {
	return c.callWith(true, method, path, q, payload, out)
}

// callWith is call, which does not go over another route if not replay.
func (c *Client) callWith(replay bool, method, path string, q url.Values, payload any, out any) error {
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	return c.callBody(replay, method, path, q, func(token string) (*http.Request, error) {
		return c.request(method, path, q, body, token)
	}, out)
}

// callBody asks as build makes the request, signed in, and once more signed
// in again if the session ended.
func (c *Client) callBody(replay bool, method, path string, q url.Values, build func(token string) (*http.Request, error), out any) error {
	for retry := false; ; retry = true {
		token, err := c.session()
		if err != nil {
			return err
		}
		res, err := c.sendWith(func() (*http.Request, error) { return build(token) }, replay)
		if err == nil {
			err = decode(res, out)
		}
		if err == errSession && !retry {
			if err := c.refresh(token); err != nil {
				return err
			}
			continue
		}
		if err == errSession {
			return ErrBadLogin
		}
		return err
	}
}

// SetFavorite likes a song, or unlikes it.
func (c *Client) SetFavorite(ref string, like bool) error {
	path := "/favorite-track/delete"
	if like {
		path = "/favorite-track/create"
	}
	return c.call("POST", path, nil, map[string]string{"trackGUID": ref}, nil)
}

// Playlists returns the listener's playlists, without their songs.
func (c *Client) Playlists() ([]netease.Playlist, error) {
	var res struct {
		List []struct {
			GUID       string `json:"guid"`
			Name       string `json:"name"`
			CoverID    string `json:"coverId"`
			TrackCount int    `json:"trackCount"`
		} `json:"list"`
	}
	if err := c.call("GET", "/playlist/list", nil, nil, &res); err != nil {
		return nil, err
	}
	lists := make([]netease.Playlist, 0, len(res.List))
	for _, p := range res.List {
		if p.GUID == "" {
			continue
		}
		lists = append(lists, netease.Playlist{
			ID: ID(p.GUID), Ref: p.GUID, Name: p.Name, CoverURL: coverURL(p.CoverID), TrackCount: p.TrackCount,
		})
	}
	return lists, nil
}

// Stream returns where a track streams from. It asks for its first byte,
// which tells how long it is, and that the session still lets it play.
func (c *Client) Stream(t *netease.Track) (*netease.Stream, error) {
	q := url.Values{"guid": {t.Ref}}
	for retry := false; ; retry = true {
		token, err := c.session()
		if err != nil {
			return nil, err
		}
		res, err := c.send(func() (*http.Request, error) {
			req, err := c.request("GET", "/track/stream", q, nil, token)
			if err == nil {
				req.Header.Del("Accept")
				req.Header.Set("Range", "bytes=0-0")
			}
			return req, err
		})
		if err != nil {
			return nil, err
		}
		res.Body.Close()
		if (res.StatusCode == 401 || res.StatusCode == 403) && !retry {
			if err := c.refresh(token); err != nil {
				return nil, err
			}
			continue
		}
		if res.StatusCode != http.StatusPartialContent && res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fnmusic: %s", res.Status)
		}
		return &netease.Stream{
			// Where the NAS was found, which may be other than before.
			URL:    res.Request.URL.String(),
			Size:   totalSize(res),
			Type:   t.Format,
			Level:  levelOf(t.Format),
			Header: func() http.Header { return c.mediaHeader(q) },
		}, nil
	}
}

// levelOf is the level of a stream of format, which says lossless ones.
func levelOf(format string) string {
	if format == "flac" {
		return "lossless"
	}
	return "exhigh"
}

// totalSize reads how long the whole of what a reply is a part of is.
func totalSize(res *http.Response) int64 {
	if _, total, ok := strings.Cut(res.Header.Get("Content-Range"), "/"); ok {
		if n, err := strconv.ParseInt(total, 10, 64); err == nil {
			return n
		}
	}
	if res.StatusCode == http.StatusOK {
		return max(res.ContentLength, 0)
	}
	return 0
}

// mediaHeader makes the headers of a request for media, signed anew:
// the server takes a signature of the moment.
func (c *Client) mediaHeader(q url.Values) http.Header {
	c.mu.Lock()
	token := c.token
	c.mu.Unlock()
	req, err := c.request("GET", "/track/stream", q, nil, token)
	if err != nil {
		return nil
	}
	req.Header.Del("Accept")
	return req.Header
}

// Cover returns the picture of a cover, size pixels across.
func (c *Client) Cover(id string, size int) ([]byte, error) {
	q := url.Values{"coverId": {id}, "size": {strconv.Itoa(max(64, min(size, 2048)))}}
	for retry := false; ; retry = true {
		token, err := c.session()
		if err != nil {
			return nil, err
		}
		res, err := c.send(func() (*http.Request, error) {
			req, err := c.request("GET", "/static/cover", q, nil, token)
			if err == nil {
				req.Header.Del("Accept")
			}
			return req, err
		})
		if err != nil {
			return nil, err
		}
		if (res.StatusCode == 401 || res.StatusCode == 403) && !retry {
			res.Body.Close()
			if err := c.refresh(token); err != nil {
				return nil, err
			}
			continue
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fnmusic: %s", res.Status)
		}
		b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
		// An error comes as JSON, not as the picture.
		if err == nil && strings.Contains(res.Header.Get("Content-Type"), "json") {
			return nil, errors.New("fnmusic: no cover")
		}
		return b, err
	}
}

// Lyrics returns the lyrics of a track as LRC, or "" for none.
func (c *Client) Lyrics(ref string) (string, error) {
	var res lyricList
	if err := c.call("GET", "/lyric/list", url.Values{"trackGUID": {ref}}, nil, &res); err != nil {
		return "", err
	}
	return chooseLyrics(res), nil
}

// ID is the app's number for something of the service, from its GUID:
// below zero, where NetEase's are not.
func ID(guid string) int64 {
	h := fnv.New64a()
	h.Write([]byte(guid))
	return -int64(h.Sum64()>>2) - 1000
}

// Special playlists of the library, which the service does not list.
const (
	LibraryID   int64 = -1
	FavoritesID int64 = -2
	RecentID    int64 = -4 // what was played last, as the service keeps it
)

// Covers are named in URLs of their own: fnmusic://cover/ID.
const coverScheme = "fnmusic://cover/"

func coverURL(id string) string {
	if id == "" {
		return ""
	}
	return coverScheme + url.PathEscape(id)
}

// def is the client covers are fetched with; set it with SetDefault.
var (
	defMu sync.Mutex
	def   *Client
)

// SetDefault sets the client that fetches covers, nil for none.
func SetDefault(c *Client) {
	defMu.Lock()
	def = c
	defMu.Unlock()
}

// Current returns the client covers are fetched with, nil for none.
func Current() *Client {
	defMu.Lock()
	defer defMu.Unlock()
	return def
}

// Fetch returns the picture at url: a cover of the service, or else
// whatever the NetEase image CDN serves there.
func Fetch(u string) ([]byte, error) {
	rest, ok := strings.CutPrefix(u, coverScheme)
	if !ok {
		return netease.Fetch(u)
	}
	c := Current()
	if c == nil {
		return nil, ErrSignedOut
	}
	id, query, _ := strings.Cut(rest, "?")
	id, err := url.PathUnescape(id)
	if err != nil {
		return nil, err
	}
	size := 640
	if v, err := url.ParseQuery(query); err == nil {
		if n, err := strconv.Atoi(v.Get("size")); err == nil {
			size = n
		}
	}
	return c.Cover(id, size)
}
