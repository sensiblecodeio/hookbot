package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	TEST_GITHUB_SECRET = "github_secret"
	TEST_KEY           = "key"
)

func MakePubRequest(url, body string) (w *httptest.ResponseRecorder, r *http.Request) {
	w = httptest.NewRecorder()
	bodyReader := bytes.NewReader([]byte(body))
	r, _ = http.NewRequest("POST", "http://localhost"+url, bodyReader)
	return w, r
}

// Unsafe pub should always succeed, without credentials
func TestUnsafePub(t *testing.T) {

	run := func(iteration int) {

		w, r := MakePubRequest("/unsafe/pub/", "MESSAGE")

		var c chan []byte

		func() {
			hookbot := NewHookbot(TEST_KEY, TEST_GITHUB_SECRET)
			defer hookbot.Shutdown()

			msgs := hookbot.Add("/unsafe/")
			defer hookbot.Del(msgs)
			c = msgs.c

			hookbot.ServeHTTP(w, r)
		}()

		if w.Code != http.StatusOK {
			t.Errorf("Status code != 200 (= %v)", w.Code)
		}

		// Message should have been delivered by the time we see
		// hookbot.Shutdown().
		select {
		case <-c:
		default:
			t.Fatalf("Message not delivered (iteration %v)", iteration)
		}
	}

	// Run the test repeatedly to search for races.
	for i := 0; i < 10; i++ {
		run(i)
	}
}

// Bad authentication should cause a 404 not found.
func TestAuthMissingFail(t *testing.T) {
	w, r := MakePubRequest("/pub/", "MESSAGE")

	func() {
		hookbot := NewHookbot(TEST_KEY, TEST_GITHUB_SECRET)
		defer hookbot.Shutdown()

		hookbot.ServeHTTP(w, r)
	}()

	if w.Code != http.StatusNotFound {
		t.Errorf("Status code != 404 (= %v)", w.Code)
	}
}

// Invalid secret authentication should return 404 Not Found.
func TestAuthInvalidSecret(t *testing.T) {
	w, r := MakePubRequest("/pub/", "MESSAGE")

	token := Sha1HMAC(TEST_KEY, "/pub/not/the/same/as/above") // bad secret
	r.SetBasicAuth(token, "")

	func() {
		hookbot := NewHookbot(TEST_KEY, TEST_GITHUB_SECRET)
		defer hookbot.Shutdown()

		hookbot.ServeHTTP(w, r)
	}()

	if w.Code != http.StatusNotFound {
		t.Errorf("Status code != 404 (= %v)", w.Code)
	}
}

// Valid secret authentication should return 200 OK
func TestAuthSuccess(t *testing.T) {
	w, r := MakePubRequest("/pub/place", "MESSAGE")

	token := Sha1HMAC(TEST_KEY, "/pub/place")
	r.SetBasicAuth(token, "")

	func() {
		hookbot := NewHookbot(TEST_KEY, TEST_GITHUB_SECRET)
		defer hookbot.Shutdown()

		hookbot.ServeHTTP(w, r)
	}()

	if w.Code != http.StatusOK {
		t.Errorf("Status code != 200 (= %v)", w.Code)
	}
}
