package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/h2non/bimg"
	"github.com/moonrhythm/parapet"
	"github.com/moonrhythm/parapet/pkg/router"
)

var httpClient = http.Client{
	Timeout: 2 * time.Second,
}

const cacheVersion = 1

var (
	// allowDomains is comma separated list of allowed domains
	allowDomains string
	// baseURL is base url when url isn't start with http(s)://
	baseURL string
)

func main() {
	flag.StringVar(&allowDomains, "allow-domains", "", "allow domains (comma separated)")
	flag.StringVar(&baseURL, "base-url", "", "base url when url isn't start with http(s)://")

	flag.Parse()

	log.Printf("allow domains: %s", strings.Split(allowDomains, ","))
	if err := serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
}

func serve() error {
	s := parapet.NewBackend()
	r := router.New()
	r.Handle("/", parapet.MiddlewareFunc(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			srcURL := r.URL.Query().Get("url")
			if srcURL == "" {
				http.Error(w, "url is required", http.StatusBadRequest)
				return
			}

			// detect leading url and if base url is set
			if !strings.HasPrefix(srcURL, "http") && baseURL != "" {
				joinURL, err := url.JoinPath(baseURL, srcURL)
				if err != nil {
					http.Error(w, "invalid image url", http.StatusBadRequest)
					return
				}
				srcURL = joinURL
			}

			targetURL, err := url.Parse(srcURL)
			if err != nil {
				http.Error(w, "invalid image url", http.StatusBadRequest)
				return
			}

			width, _ := strconv.Atoi(r.URL.Query().Get("w"))
			quality, _ := strconv.Atoi(r.URL.Query().Get("q"))

			// filter domain
			if allowDomains != "" && !strings.Contains(allowDomains, targetURL.Host) {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintln(w, "Domain not allowed")
				return
			}

			mimeType, _, _ := mime.ParseMediaType(r.Header.Get("Accept"))
			// generate cache key
			cacheKey := getCacheKey(srcURL, width, quality, mimeType)

			// try to read from cache
			if res, err := readImageFileSystem(cacheKey, "./cache"); err == nil && res != nil {
				sendResponse(w, res, CacheHit, nil, r)
				return
			}

			res, err := httpClient.Get(srcURL)
			if err != nil {
				http.Error(w, "cannot get image from upstream", http.StatusInternalServerError)
				return
			}

			defer res.Body.Close()
			defer io.Copy(io.Discard, res.Body)
			// we only cache response
			if res.StatusCode >= http.StatusBadRequest {
				http.Error(w, "cannot get image from upstream", http.StatusInternalServerError)
				return
			}

			var buf bytes.Buffer

			if _, err := io.Copy(&buf, res.Body); err != nil {
				http.Error(w, "cannot read image from upstream", http.StatusInternalServerError)
				return
			}

			maxAge := getMaxAge(res.Header.Get("Cache-Control"))

			metadata, err := bimg.NewImage(buf.Bytes()).Metadata()
			if err != nil {
				http.Error(w, "cannot read image metadata", http.StatusInternalServerError)
				return
			}

			// when width is not set, use the original width
			reWidth := 0
			if metadata.Size.Width > width {
				reWidth = width
			}
			resizeImg, err := bimg.Resize(buf.Bytes(), bimg.Options{
				Quality: quality,
				Width:   reWidth,
				Type:    bimg.WEBP,
			})
			if err != nil {
				http.Error(w, "cannot resize image", http.StatusInternalServerError)
				return
			}

			go func() {
				err := writeImageToFile(cacheKey, maxAge, getHash(resizeImg), resizeImg)
				if err != nil {
					log.Printf("warning cannot write image to file: %s", err)
				}
			}()

			sendResponse(w, &Response{
				buf:    buf.Bytes(),
				MaxAge: maxAge,
				ETag:   getHash(buf.Bytes()),
			}, "", nil, r)

		})
	}))
	s.Use(r)
	s.Addr = ":8080"
	log.Printf("Listening on %s", s.Addr)
	return s.ListenAndServe()
}

func getCacheKey(url string, w int, q int, mimeType string) string {
	return getHash(
		cacheVersion, url, w, q, mimeType,
	)
}

func getMaxAge(str string) int {
	cacheControl := parseCacheControl(str)
	ageStr := cacheControl["s-maxage"]
	if ageStr == "" {
		ageStr = cacheControl["max-age"]
	}
	age, err := strconv.Atoi(strings.Trim(ageStr, "\""))
	if err != nil {
		return 604800 // default cache control
	}
	return age
}

func parseCacheControl(str string) map[string]string {
	m := make(map[string]string)
	if str == "" {
		return m
	}
	directives := strings.Split(str, ",")
	for _, directive := range directives {
		parts := strings.SplitN(strings.TrimSpace(directive), "=", 2)
		key := strings.ToLower(parts[0])
		var value string
		if len(parts) > 1 {
			value = strings.ToLower(strings.Trim(parts[1], "\""))
		}
		m[key] = value
	}
	return m
}
