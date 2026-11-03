package qqmusic

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync/atomic"
)

// The authorization host can close a fresh TLS handshake with EOF. Reconnect
// once only before obtaining an HTTP connection, so no signed request has been
// sent. OAuth POSTs and requests on established connections are never replayed.
func doAuthRequest(req *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		var connected atomic.Bool
		trace := &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { connected.Store(true) }}
		request := req.Clone(httptrace.WithClientTrace(req.Context(), trace))
		response, err := authHTTP.Do(request)
		interrupted := errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
		if err == nil || attempt > 0 || req.Method != http.MethodGet || connected.Load() || !interrupted {
			return response, err
		}
	}
}
