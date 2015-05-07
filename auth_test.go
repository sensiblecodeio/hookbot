package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Unsafe pub should always succeed, without credentials
func TestUnsafePub(t *testing.T) {
	hookbot := NewHookbot("key", "github_secret")

	msgs := hookbot.Add("/unsafe/")
	defer hookbot.Del(msgs)

	w := httptest.NewRecorder()

	body := bytes.NewReader([]byte("MESSAGE"))
	r, _ := http.NewRequest("POST", "http://localhost/unsafe/pub/", body)

	hookbot.ServeHTTP(w, r)

	hookbot.Shutdown()

	if w.Code != http.StatusOK {
		t.Errorf("Status code != 200 (= %v)", w.Code)
	}

	select {
	case <-msgs.c:
	default:
		t.Error("Message not delivered")
	}
}
