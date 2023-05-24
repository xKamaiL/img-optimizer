package main

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type CacheKind string

const (
	CacheHit  CacheKind = "HIT"
	CacheMiss CacheKind = "MISS"
)

type Response struct {
	buf    []byte
	MaxAge int
	ETag   string
}

func sendResponse(w http.ResponseWriter, res *Response, cache CacheKind, extraHeaders map[string]string) {
	defer w.(http.Flusher).Flush()
	maxAge := res.MaxAge

	w.Header().Add("Vary", "Accept")
	w.Header().Add("Content-Type", "image/webp")
	w.Header().Add("Cache-Control", fmt.Sprintf(
		"public, max-age=%d, must-revalidate, stale-while-revalidate=86400, stale-if-error=604800",
		maxAge,
	))
	w.Header().Add("CDN-Cache-Control", fmt.Sprintf("max-age=%d", maxAge))

	w.Header().Add("Content-Length", strconv.Itoa(len(res.buf)))
	w.Header().Set("Expires", time.Now().Add(time.Duration(maxAge)*time.Second).Format(http.TimeFormat))
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
