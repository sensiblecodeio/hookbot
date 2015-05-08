package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
)

func SecureEqual(x, y string) bool {
	if subtle.ConstantTimeCompare([]byte(x), []byte(y)) == 1 {
		return true
	}
	return false
}

func (h *Hookbot) IsGithubKeyOK(w http.ResponseWriter, r *http.Request) bool {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Not Authorized", http.StatusUnauthorized)
	}

	r.Body = ioutil.NopCloser(bytes.NewReader(body))

	expected := fmt.Sprintf("sha1=%v", Sha1HMAC(h.github_secret, string(body)))

	return SecureEqual(r.Header.Get("X-Hub-Signature"), expected)
}

func Sha1HMAC(key, payload string) string {
	mac := hmac.New(sha1.New, []byte(key))
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func (h *Hookbot) IsKeyOK(w http.ResponseWriter, r *http.Request) bool {

	if _, ok := r.Header["X-Hub-Signature"]; ok {
		if !h.IsGithubKeyOK(w, r) {
			return false
		}
		return true
	}

	authorization := r.Header.Get("Authorization")
	fields := strings.Fields(authorization)

	if len(fields) != 2 {
		return false
	}

	authType, givenKey := fields[0], fields[1]

	var givenMac string

	switch strings.ToLower(authType) {
	default:
		return false // Not understood
	case "basic":
		var ok bool
		givenMac, _, ok = r.BasicAuth()
		if !ok {
			return false
		}

	case "bearer":
		givenMac = givenKey // No processing required
	}

	expectedMac := Sha1HMAC(h.key, r.URL.Path)

	if !SecureEqual(givenMac, expectedMac) {
		return false
	}

	return true
}

var UnsafeURI = regexp.MustCompile("^/unsafe/(pub|sub)/.*")

// Unsafe requests are those with URIs which have /unsafe/ as the second
// path component.
func IsUnsafeRequest(r *http.Request) bool {
	return UnsafeURI.MatchString(r.URL.Path)
}

func RequireUnsafeHeader(wrapped http.Handler) http.HandlerFunc {
	const ErrMsg = "400 Bad Request. X-Hookbot-Unsafe-Is-Ok header required."

	return func(w http.ResponseWriter, r *http.Request) {

		values, have_unsafe_header := r.Header["X-Hookbot-Unsafe-Is-Ok"]

		if IsUnsafeRequest(r) {
			// "X-Hookbot-Unsafe-Is-Ok" header required
			if !have_unsafe_header {
				http.Error(w, ErrMsg, http.StatusBadRequest)
				return
			}

			// Unsafe URLs can be published to by anyone on the internet
			// without a valid key and it is *your* responsibility to check
			// the key. This is so that the security checking can happen
			// in another component (e.g, a thing that understand's github's
			// signing mechanism). The header is required so that people
			// don't mistakenly specify an unsafe URL in a component which
			// must not use one.
			if values[0] != "I understand the security implications" {
				http.Error(w, ErrMsg, http.StatusBadRequest)
				return
			}
		}

		wrapped.ServeHTTP(w, r)
	}
}

func (h *Hookbot) KeyChecker(wrapped http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if IsUnsafeRequest(r) {
			// Unsafe requests don't check keys. See UnsafeChecker.
			wrapped.ServeHTTP(w, r)
			return
		}

		if !h.IsKeyOK(w, r) {
			http.NotFound(w, r)
			return
		}

		wrapped.ServeHTTP(w, r)
	}
}
