package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/h2non/bimg"
	"github.com/moonrhythm/parapet"
	"github.com/moonrhythm/parapet/pkg/router"
)

var httpClient = http.Client{
	Timeout: 1 * time.Second,
}
var (
	allowDomains string
	baseURL      string
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
			// check url
			if !strings.HasPrefix(srcURL, "http") {
				joinURL, err := url.JoinPath(baseURL, srcURL)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprintln(w, "Invalid url: "+err.Error())
					return
				}
				srcURL = joinURL
			}
			targetURL, err := url.Parse(srcURL)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintln(w, "Invalid url")
				return
			}

			width, _ := strconv.Atoi(r.URL.Query().Get("w"))
			quality, _ := strconv.Atoi(r.URL.Query().Get("q"))

			if allowDomains != "" && !strings.Contains(allowDomains, targetURL.Host) {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintln(w, "Domain not allowed")
				return
			}
			mimeType, _, _ := mime.ParseMediaType(r.Header.Get("Accept"))

			cacheKey := getCacheKey(srcURL, width, quality, mimeType)

			// check from cache ?
			bufFromCache, err2 := readImageFileSystem(w, cacheKey, "./cache")
			if err2 == nil && bufFromCache != nil {
				w.Write(bufFromCache)
				return
			}

			res, err := httpClient.Get(srcURL)
			if err != nil {
				fmt.Fprintln(w, "cannot get upstream url")
				return
			}
			defer res.Body.Close()
			defer io.Copy(io.Discard, res.Body)
			var buf bytes.Buffer

			if _, err := io.Copy(&buf, res.Body); err != nil {
				fmt.Fprintln(w, "cannot read byte upstream response")
				return
			}
			maxAge := getMaxAge(res.Header.Get("Cache-Control"))
			metadata, err := bimg.NewImage(buf).Metadata()
			if err != nil {
				fmt.Fprintln(w, "cannot get metadata")
				return
			}

			reWidth := 0
			if metadata.Size.Width > width {
				reWidth = width
			}
			resizeImg, err := bimg.Resize(buf, bimg.Options{
				Quality: quality,
				Width:   reWidth,
				Type:    bimg.WEBP,
			})
			if err != nil {
				fmt.Fprintln(w, "cannot resize image")
				return
			}
			//defer fmt.Printf("resize image %d bytes, url = %s, cache = %s\n", len(resizeImg), url, cacheKey)

			go func() {
				err := writeImageToFile(cacheKey, maxAge, getHash(resizeImg), resizeImg)
				if err != nil {
					log.Printf("warning cannot write image to file: %s", err)
				}
			}()

			w.Header().Set("Content-Length", strconv.Itoa(len(resizeImg)))
			w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", maxAge))
			w.WriteHeader(http.StatusOK)
			w.Write(resizeImg)

			// do we have to defer ?

		})
	}))
	s.Use(r)
	s.Addr = ":8080"
	log.Printf("Listening on %s", s.Addr)
	return s.ListenAndServe()
}

func getCacheKey(url string, w int, q int, mimeType string) string {
	return getHash(
		1, url, w, q, mimeType,
	)
}

func writeImageToFile(cacheKey string, maxAge int, etag string, buf []byte) error {
	// get extension from content type
	ext, err := mime.ExtensionsByType("image/webp")
	if err != nil {
		return err
	}
	dir := filepath.Join("./cache", cacheKey)

	fileName := fmt.Sprintf("%d.%+v.%s%s", maxAge, int64(maxAge)+time.Now().Unix(), etag, ext[0])
	log.Printf("write image to file %s", fileName)
	targetFilePath := filepath.Join(dir, fileName)

	// Create the target directory if it does not exist
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, fs.ModePerm); err != nil {
			return err
		}
	}
	// Write the image buffer to the target file
	f, err := os.Create(targetFilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(buf); err != nil {
		return err
	}
	return nil
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

func getMaxAge(str string) int {
	cacheControl := parseCacheControl(str)
	ageStr := cacheControl["s-maxage"]
	if ageStr == "" {
		ageStr = cacheControl["max-age"]
	}
	ageStr = strings.Trim(ageStr, "\"")
	age, err := strconv.Atoi(ageStr)
	if err != nil {
		return 0
	}
	return age
}

func getHash(items ...interface{}) string {
	hash := sha256.New()
	for _, item := range items {
		switch v := item.(type) {
		case string:
			hash.Write([]byte(v))
		case int:
			hash.Write([]byte(strconv.Itoa(v)))
		case []byte:
			hash.Write(v)
		default:
			// do nothing
		}
	}
	hashBytes := hash.Sum(nil)
	// See https://en.wikipedia.org/wiki/Base64#Filenames
	hashString := base64.URLEncoding.EncodeToString(hashBytes)
	return hashString
}

func readImageFileSystem(w http.ResponseWriter, cacheKey string, cacheDirectory string) ([]byte, error) {
	now := time.Now().Unix()

	requestedDirectory := filepath.Join(cacheDirectory, cacheKey)
	files, err := os.ReadDir(requestedDirectory)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		//  14400.790149400.lb8rhuuM92bhYv2KvdczyjnmOqtYouQLs6UF_uHFHGY=.webp
		// [max-age, expired_at, etag, ext]
		fileName := file.Name()
		ext := filepath.Ext(fileName)
		if ext == "" {
			log.Printf("warning: invalid file name: %s", fileName)
			continue
		}
		parts := strings.Split(fileName[:len(fileName)-len(ext)], ".")
		if len(parts) < 3 {
			log.Printf("warning: invalid file name: %s", fileName)
			continue
		}
		expireAtString := parts[1]
		filePath := filepath.Join(requestedDirectory, fileName)

		expireAt, err := strconv.ParseInt(expireAtString, 10, 64)
		if err != nil {
			continue
		}
		if expireAt < now {
			go os.Remove(filePath)
			return nil, errors.New("not found")
		}
		return os.ReadFile(filePath)
	}

	return nil, nil
}
