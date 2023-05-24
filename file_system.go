package main

import (
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type FileMetadata struct {
	MaxAge    int
	ExpireAt  int64
	Etag      string
	Extension string
}

func getMetadataFromFilename(filename string) (*FileMetadata, error) {
	meta := &FileMetadata{}
	ext := filepath.Ext(filename)
	if ext == "" {
		return nil, errors.New("invalid file extension")
	}
	meta.Extension = ext

	parts := strings.Split(filename[:len(filename)-len(ext)], ".")
	if len(parts) < 3 {
		return nil, errors.New("invalid file name part is not enough")
	}

	// `${maxAge}.${expireAt}.${etag}.${extension}`

	expireAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, err
	}
	meta.ExpireAt = expireAt

	maxAge, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, err
	}
	meta.MaxAge = maxAge
	meta.Etag = parts[2]
	return meta, nil
}

func readImageFileSystem(cacheKey string, cacheDirectory string) (*Response, error) {
	now := time.Now().Unix()

	requestedDirectory := filepath.Join(cacheDirectory, cacheKey)
	files, err := os.ReadDir(requestedDirectory)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		metadata, err := getMetadataFromFilename(file.Name())
		if err != nil {
			return nil, err
		}
		filePath := filepath.Join(requestedDirectory, file.Name())
		if metadata.ExpireAt < now {
			go os.Remove(filePath)
			return nil, errors.New("cache not found")
		}

		buf, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}

		return &Response{
			buf:         buf,
			ContentType: http.DetectContentType(buf),
			MaxAge:      metadata.MaxAge,
			ETag:        metadata.Etag,
		}, nil
	}

	return nil, errors.New("cache not found")
}

func writeImageToFile(cacheKey string, maxAge int, etag string, buf []byte) error {
	// get an extension from content type
	ext, err := mime.ExtensionsByType("image/webp")
	if err != nil {
		return err
	}
	dir := filepath.Join("./cache", cacheKey)

	fileName := fmt.Sprintf("%d.%+v.%s%s", maxAge, int64(maxAge)+time.Now().Unix(), etag, ext[0])
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
