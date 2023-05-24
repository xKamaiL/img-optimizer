package main

import (
	"crypto/sha256"
	"encoding/base64"
	"strconv"
)

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
	return base64.URLEncoding.EncodeToString(hashBytes)
}
