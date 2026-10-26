package fnmusic

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FN Connect is Feiniu's way to reach a NAS by its FN ID, from outside its
// network: a lookup service tells where the NAS may be found (its addresses
// at home, on the internet, its DDNS names, and a relay of Feiniu's), and
// the app tries them in that order, taking the first that answers as the
// music service.

// What goes wrong in finding a NAS by its FN ID, in the words to show.
var (
	ErrInvalidID          = errors.New("FN ID 格式不正确")
	ErrServerNotFound     = errors.New("未找到这个 FN ID 对应的设备")
	ErrInvalidResponse    = errors.New("FN Connect 返回了无法识别的响应")
	ErrAccessCodeRequired = errors.New("这台设备启用了访问码，请填写 FN Connect 访问码")
	ErrAccessCodeRejected = errors.New("FN Connect 访问码不正确")
	ErrDiscoveryDown      = errors.New("FN Connect 暂时不可用，请稍后再试")
	ErrMusicUnavailable   = errors.New("已找到设备，但飞牛音乐服务不可用，请确认已安装并启动")
	ErrUnreachable        = errors.New("无法连接到设备，请检查网络")
	errNoCandidates       = ErrInvalidResponse
	errNoRoam             = errors.New("fnmusic: 漫游暂时没有歌曲")
)

// Where FN Connect is, and how long to wait for what it points to.
var (
	lookupURL         = "https://5ddd.com/api/v1/fn/con"
	lookupPath        = "/api/v1/fn/con"
	lookupKey         = "zIGtkc3dqZnJpd29qZXJqa2w7c"
	probeTimeout      = 2 * time.Second
	relayProbeTimeout = 10 * time.Second
	lookupTimeout     = 10 * time.Second
	relayDomains      = []string{"5ddd.com", "fnos.net"}
	defaultHTTPPort   = 5666
	defaultHTTPSPort  = 5667
)

// UserError reports whether err says in words what went wrong, to show as
// it is.
func UserError(err error) bool {
	for _, e := range []error{ErrBadLogin, ErrInvalidID, ErrServerNotFound, ErrInvalidResponse,
		ErrAccessCodeRequired, ErrAccessCodeRejected, ErrDiscoveryDown, ErrMusicUnavailable, ErrUnreachable} {
		if err == e {
			return true
		}
	}
	return false
}

// FNID reads an FN ID from what a listener types: the ID, or its address
// <id>.5ddd.com. It is 6 to 63 letters, digits and hyphens.
func FNID(raw string) (string, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", false
	}
	if !strings.Contains(v, "://") {
		v = strings.TrimSuffix(strings.ToLower(v), ".5ddd.com")
	} else if u, err := url.Parse(v); err == nil && strings.HasSuffix(strings.ToLower(u.Hostname()), ".5ddd.com") &&
		(u.Path == "" || u.Path == "/") {
		v = strings.TrimSuffix(strings.ToLower(u.Hostname()), ".5ddd.com")
	}
	if n := len(v); n < 6 || n > 63 || v[0] == '-' || v[n-1] == '-' {
		return "", false
	}
	for _, r := range v {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-') {
			return "", false
		}
	}
	return strings.ToLower(v), true
}

// endpoint is where the service was found: the base of its API, and
// whether that is Feiniu's relay, which asks for a cookie to say so.
type endpoint struct {
	base  *url.URL
	relay bool
}

// endpoint returns where the service is, finding it first if it is not
// known: in ModeFNConnect, the first time and after Invalidate.
func (c *Client) endpoint() (endpoint, error) {
	c.epMu.Lock()
	ep := c.ep
	c.epMu.Unlock()
	if ep != nil {
		return *ep, nil
	}
	// One finds it, and the others wait to be told.
	c.findMu.Lock()
	defer c.findMu.Unlock()
	c.epMu.Lock()
	ep = c.ep
	c.epMu.Unlock()
	if ep != nil {
		return *ep, nil
	}
	found, err := c.resolve()
	if err != nil {
		return endpoint{}, err
	}
	c.epMu.Lock()
	c.ep = &found
	c.epMu.Unlock()
	return found, nil
}

// Invalidate forgets where an FN Connect NAS was found, to find it anew
// with the next request: the network changed, or the route stopped
// answering.
func (c *Client) Invalidate() {
	if c.cfg.Mode != ModeFNConnect {
		return
	}
	c.epMu.Lock()
	c.ep = nil
	c.epMu.Unlock()
}

// Route says how an FN Connect NAS was reached, "" if not yet, or not
// by FN Connect.
func (c *Client) Route() string {
	c.epMu.Lock()
	defer c.epMu.Unlock()
	switch {
	case c.cfg.Mode != ModeFNConnect || c.ep == nil:
		return ""
	case c.ep.relay:
		return "中继"
	}
	return "直连"
}

// Connect finds the service, which signing in would do as well: for a
// dialog to say what is wrong before it asks for more.
func (c *Client) Connect() error {
	_, err := c.endpoint()
	return err
}

// routeFailure reports whether err says the service could not be got
// through to, which another route may: not that it answered no.
func routeFailure(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	var op *net.OpError
	var cert x509.UnknownAuthorityError
	return errors.As(err, &op) || errors.As(err, &cert) || errors.Is(err, io.ErrUnexpectedEOF)
}

// newHTTPClient makes the client of the service. A NAS at home answers
// over HTTPS with a certificate of its own, to an address no authority
// names: so addresses given as numbers are not verified, as a browser lets
// its user go on, and names are.
func newHTTPClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
		VerifyConnection:   verifyNamed,
	}
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: tr,
		// Never follow what is sent elsewhere, with a session in tow.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.Host != via[0].URL.Host {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

// verifyNamed verifies a certificate as usual, but for a server given as
// an IP address.
func verifyNamed(cs tls.ConnectionState) error {
	if cs.ServerName == "" || net.ParseIP(cs.ServerName) != nil {
		return nil
	}
	if len(cs.PeerCertificates) == 0 {
		return errors.New("no certificate")
	}
	inter := x509.NewCertPool()
	for _, cert := range cs.PeerCertificates[1:] {
		inter.AddCert(cert)
	}
	_, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{DNSName: cs.ServerName, Intermediates: inter})
	return err
}

// candidate is an address the NAS may be found at.
type candidate struct {
	base  *url.URL // as far as the host and port
	relay bool
}

// lookup asks FN Connect where the NAS with an FN ID may be, in groups
// to try in turn.
func (c *Client) lookup(id string) ([][]candidate, error) {
	body, _ := json.Marshal(map[string]string{"fnId": id})
	req, err := http.NewRequest("POST", lookupURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	nonce := fmt.Sprintf("%06d", 100000+randomN(900000))
	req.Header.Set("authx", signWith(lookupKey, lookupPath, string(body), nonce, time.Now().UnixMilli()))
	client := &http.Client{Timeout: lookupTimeout, Transport: c.http.Transport}
	res, err := client.Do(req)
	if err != nil {
		return nil, ErrDiscoveryDown
	}
	defer res.Body.Close()
	var env struct {
		Code json.Number `json:"code"`
		Data *struct {
			IPv4       []string `json:"ipv4"`
			IPv6       []string `json:"ipv6"`
			PublicIPv4 []string `json:"publicIpv4"`
			PublicIPv6 []string `json:"publicIpv6"`
			FN         []string `json:"fn"`
			DDNS       []string `json:"ddns"`
			Port       struct {
				HTTPS int `json:"httpsPort"`
				HTTP  int `json:"httpPort"`
			} `json:"port"`
		} `json:"data"`
	}
	if res.StatusCode/100 != 2 || json.NewDecoder(res.Body).Decode(&env) != nil {
		return nil, ErrInvalidResponse
	}
	switch code, _ := env.Code.Int64(); code {
	case 0:
	case 404, 1001, 1004, 3000006, 3000037:
		return nil, ErrServerNotFound
	case 3000009:
		return nil, ErrDiscoveryDown
	default:
		return nil, ErrInvalidResponse
	}
	if env.Data == nil {
		return nil, ErrServerNotFound
	}
	d := env.Data
	httpsPort, httpPort := validPort(d.Port.HTTPS), validPort(d.Port.HTTP)
	ddnsPort := httpsPort
	if httpsPort == 0 {
		httpsPort = defaultHTTPSPort
	}
	if httpPort == 0 {
		httpPort = defaultHTTPPort
	}

	var local, localTLS, pub6, pub4, ddns, relay []candidate
	for _, h := range append(append([]string(nil), d.IPv4...), d.IPv6...) {
		if isPrivate(h) {
			local = append(local, ipCandidate(h, httpPort, "http"))
		}
		localTLS = append(localTLS, ipCandidate(h, httpsPort, "https"))
	}
	for _, h := range d.PublicIPv6 {
		pub6 = append(pub6, ipCandidate(h, httpsPort, "https"))
	}
	for _, h := range d.PublicIPv4 {
		pub4 = append(pub4, ipCandidate(h, httpsPort, "https"))
	}
	for _, h := range d.DDNS {
		ddns = append(ddns, ddnsCandidate(h, ddnsPort))
	}
	relays := d.FN
	if len(relays) == 0 {
		relays = []string{id + ".5ddd.com"}
	}
	for _, h := range relays {
		relay = append(relay, relayCandidate(h))
	}
	var groups [][]candidate
	for _, g := range [][]candidate{local, localTLS, pub6, pub4, ddns, relay} {
		if g = unique(g); len(g) > 0 {
			groups = append(groups, g)
		}
	}
	if len(groups) == 0 {
		return nil, errNoCandidates
	}
	return groups, nil
}

func validPort(p int) int {
	if p < 1 || p > 65535 {
		return 0
	}
	return p
}

// isPrivate reports whether host is an address of a private network.
func isPrivate(host string) bool {
	a, err := netip.ParseAddr(strings.TrimSpace(host))
	return err == nil && (a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast())
}

// ipCandidate is an address given as an IP, which candidate drops if it
// is not one.
func ipCandidate(host string, port int, scheme string) candidate {
	host = strings.TrimSpace(host)
	a, err := netip.ParseAddr(host)
	if err != nil {
		return candidate{}
	}
	return candidate{base: hostURL(scheme, a.String(), port)}
}

func ddnsCandidate(host string, port int) candidate {
	host = strings.ToLower(strings.TrimSpace(host))
	// A name, not a URL or a path.
	if host == "" || len(host) > 253 {
		return candidate{}
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return candidate{}
		}
		for _, r := range label {
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r == '-') {
				return candidate{}
			}
		}
	}
	return candidate{base: hostURL("https", host, port)}
}

func relayCandidate(raw string) candidate {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return candidate{}
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return candidate{}
	}
	host := strings.ToLower(u.Hostname())
	for _, d := range relayDomains {
		if strings.HasSuffix(host, "."+d) {
			return candidate{base: hostURL("https", host, 0), relay: true}
		}
	}
	return candidate{}
}

// hostURL is scheme://host[:port], without the port when it is the
// scheme's own.
func hostURL(scheme, host string, port int) *url.URL {
	h := host
	if strings.Contains(host, ":") {
		h = "[" + host + "]"
	}
	if port != 0 && !(scheme == "http" && port == 80) && !(scheme == "https" && port == 443) {
		h += ":" + strconv.Itoa(port)
	}
	return &url.URL{Scheme: scheme, Host: h}
}

// unique drops what is no candidate and what is there twice.
func unique(cs []candidate) []candidate {
	seen := map[string]bool{}
	var out []candidate
	for _, c := range cs {
		if c.base == nil || seen[strings.ToLower(c.base.String())] {
			continue
		}
		seen[strings.ToLower(c.base.String())] = true
		out = append(out, c)
	}
	return out
}

// probeResult is what trying one address came to.
type probeResult int

const (
	probeFailed probeResult = iota
	probeOK
	probeChallenge // it wants an access code
	probeNoMusic   // it answered, but is no music service
)

// resolve finds the NAS of the FN ID: it asks FN Connect where it may be,
// and tries those addresses, group by group, all of a group at once, until
// one answers as the music service.
func (c *Client) resolve() (endpoint, error) {
	groups, err := c.lookup(c.cfg.Address)
	if err != nil {
		return endpoint{}, err
	}
	challenge, noMusic := false, false
	for _, group := range groups {
		results := make([]probeResult, len(group))
		var wg sync.WaitGroup
		for i, cand := range group {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i] = c.probe(cand)
			}()
		}
		wg.Wait()
		for i, r := range results {
			switch r {
			case probeOK:
				u := *group[i].base
				u.Path = apiPath
				return endpoint{base: &u, relay: group[i].relay}, nil
			case probeChallenge:
				challenge = true
			case probeNoMusic:
				noMusic = true
			}
		}
	}
	switch {
	case challenge && c.cfg.AccessCode != "":
		return endpoint{}, ErrAccessCodeRejected
	case challenge:
		return endpoint{}, ErrAccessCodeRequired
	case noMusic:
		return endpoint{}, ErrMusicUnavailable
	}
	return endpoint{}, ErrUnreachable
}

// probe asks an address whether it is the music service: first whether it
// lets the access code through, then for its config, signed as the web
// player signs.
func (c *Client) probe(cand candidate) probeResult {
	timeout := probeTimeout
	if cand.relay {
		timeout = relayProbeTimeout
	}
	client := &http.Client{Timeout: timeout, Transport: c.http.Transport, CheckRedirect: c.http.CheckRedirect}
	headers := func(req *http.Request) {
		if cand.relay {
			req.Header.Set("Cookie", "mode=relay")
		}
		if c.cfg.AccessCode != "" {
			req.Header.Set("x-access-code", base64.StdEncoding.EncodeToString([]byte(c.cfg.AccessCode)))
			req.Header.Set("x-access-source", "app")
		}
	}

	req, err := http.NewRequest("GET", cand.base.String()+"/access_code_verify", nil)
	if err != nil {
		return probeFailed
	}
	headers(req)
	res, err := client.Do(req)
	if err != nil {
		return probeFailed
	}
	res.Body.Close()
	switch {
	case res.StatusCode/100 == 2 || res.StatusCode == 404:
		// Older NAS software has no such address; the config says.
	case res.StatusCode == 401 || res.StatusCode == 403 || res.StatusCode == 429:
		return probeChallenge
	default:
		return probeFailed
	}

	path := apiPath + "/sys/config"
	req, err = http.NewRequest("GET", cand.base.String()+path, nil)
	if err != nil {
		return probeFailed
	}
	headers(req)
	req.Header.Set("Accept", "application/json")
	nonce := fmt.Sprintf("%06d", 100000+randomN(900000))
	req.Header.Set("authx", signature(path, "", nonce, time.Now().UnixMilli()))
	res, err = client.Do(req)
	if err != nil {
		return probeFailed
	}
	defer res.Body.Close()
	var env struct {
		Code json.Number `json:"code"`
	}
	if res.StatusCode/100 != 2 || json.NewDecoder(res.Body).Decode(&env) != nil {
		return probeNoMusic
	}
	if code, _ := env.Code.Int64(); code != 0 && code != 200 {
		return probeNoMusic
	}
	return probeOK
}
