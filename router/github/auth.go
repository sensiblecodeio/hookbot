package github

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

func Sha1HMAC(key, payload string) string {
	mac := hmac.New(sha1.New, []byte(key))
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func SecureEqual(x, y string) bool {
	if subtle.ConstantTimeCompare([]byte(x), []byte(y)) == 1 {
		return true
	}
	return false
}

func IsValidGithubSignature(secret string, r *http.Request) bool {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return false
	}

	r.Body = ioutil.NopCloser(bytes.NewReader(body))

	expected := fmt.Sprintf("sha1=%v", Sha1HMAC(secret, string(body)))
	got := r.Header.Get("X-Hub-Signature")

	log.Printf("Expected = %v got = %v", expected, got)

	return SecureEqual(expected, got)
}
