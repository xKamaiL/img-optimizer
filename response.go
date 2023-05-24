package main

import (
	"fmt"
	"net/http"
	"strconv"
)

type CacheKind string

const (
	CacheHit  CacheKind = "HIT"
	CacheMiss CacheKind = "MISS"
)

type Response struct {
	buf         []byte
	ContentType string
	MaxAge      int
	ETag        string
}

func sendResponse(w http.ResponseWriter, res *Response, cache CacheKind, extraHeaders map[string]string) {
	defer w.(http.Flusher).Flush()
	maxAge := res.MaxAge

	w.Header().Set("Vary", "Accept")
	w.Header().Set("Content-Type", res.ContentType)
	w.Header().Set("Cache-Control", fmt.Sprintf(
		"public, max-age=%d, must-revalidate",
		maxAge,
	))
	w.Header().Set("CDN-Cache-Control", fmt.Sprintf("max-age=%d", maxAge))

	w.Header().Set("Content-Length", strconv.Itoa(len(res.buf)))

	w.Header().Set("Content-Security-Policy", "script-src 'none'; frame-src 'none'; sandbox;")
	w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
	w.Header().Set("etag", res.ETag)
	w.Header().Set("X-SveltekitImage-Cache", string(cache))

	w.Write(res.buf)

	if extraHeaders == nil {
		return
	}

	for k, v := range extraHeaders {
		w.Header().Set(k, v)
	}

}
