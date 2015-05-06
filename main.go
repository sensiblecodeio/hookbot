package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/net/websocket"
)

func main() {
	var (
		addr          = flag.String("addr", ":8080", "address to listen on")
		key           = os.Getenv("HOOKBOT_KEY")
		github_secret = os.Getenv("HOOKBOT_GITHUB_SECRET")
	)
	flag.Parse()

	if key == "" || github_secret == "" {
		log.Fatalln("Error: HOOKBOT_KEY or HOOKBOT_GITHUB_SECRET not set")
	}

	hookbot := NewHookbot(key, github_secret)
	http.Handle("/", hookbot)
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "OK")
	})

	log.Println("Listening on", *addr)
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal(err)
	}
}

type Hookbot struct {
	key, github_secret string

	http.Handler

	message                  chan []byte
	addListener, delListener chan chan []byte
}

func NewHookbot(key, github_secret string) *Hookbot {
	h := &Hookbot{
		key: key, github_secret: github_secret,

		message:     make(chan []byte, 1),
		addListener: make(chan chan []byte, 1),
		delListener: make(chan chan []byte, 1),
	}

	mux := http.NewServeMux()
	mux.Handle("/watch/", websocket.Handler(h.ServeWatch))
	mux.HandleFunc("/notify/", h.ServeNotify)

	// Middlewares
	h.Handler = mux
	h.Handler = h.KeyChecker(h.Handler)

	go h.Loop()

	return h
}

var timeout = 1 * time.Second

func TimeoutSend(c chan []byte, m []byte) {
	select {
	case c <- m:
	case <-time.After(timeout):
	}
}

// Manage fanout from h.message onto listeners
func (h *Hookbot) Loop() {
	listeners := map[chan []byte]struct{}{}
	for {
		select {
		case m := <-h.message:
			for listener := range listeners {
				go TimeoutSend(listener, m)
			}

		case l := <-h.addListener:
			listeners[l] = struct{}{}
		case l := <-h.delListener:
			delete(listeners, l)
		}
	}
}

func (h *Hookbot) Add() chan []byte {
	c := make(chan []byte)
	h.addListener <- c
	return c
}

func (h *Hookbot) Del(c chan []byte) {
	h.delListener <- c
}

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

	mac := hmac.New(sha1.New, []byte(h.github_secret))
	mac.Reset()
	mac.Write(body)

	signature := fmt.Sprintf("sha1=%x", mac.Sum(nil))

	return SecureEqual(r.Header.Get("X-Hub-Signature"), signature)
}

func (h *Hookbot) IsKeyOK(w http.ResponseWriter, r *http.Request) bool {

	if _, ok := r.Header["X-Hub-Signature"]; ok {
		if !h.IsGithubKeyOK(w, r) {
			http.NotFound(w, r)
			return false
		}
		return true
	}

	lhs := r.Header.Get("Authorization")
	rhs := fmt.Sprintf("Bearer %v", h.key)

	if !SecureEqual(lhs, rhs) {
		http.NotFound(w, r)
		return false
	}

	return true
}

func (h *Hookbot) KeyChecker(wrapped http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.IsKeyOK(w, r) {
			return
		}

		wrapped.ServeHTTP(w, r)
	}
}

func (h *Hookbot) ServeNotify(w http.ResponseWriter, r *http.Request) {
	message, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error serving %v: %v", r.URL, err)
		return
	}
	h.message <- message
}

func (h *Hookbot) ServeWatch(conn *websocket.Conn) {

	notifications := h.Add()
	defer h.Del(notifications)

	closed := make(chan struct{})

	go func() {
		defer close(closed)
		_, _ = io.Copy(ioutil.Discard, conn)
	}()

	var message []byte

	for {
		select {
		case message = <-notifications:
		case <-closed:
			log.Printf("Client disconnected")
			return
		}

		conn.SetWriteDeadline(time.Now().Add(90 * time.Second))
		n, err := conn.Write(message)
		switch {
		case n != len(message):
			log.Printf("Short write %d != %d", n, len(message))
			return // short write
		case err == io.EOF:
			return // done
		case err != nil:
			log.Printf("Error in conn.Write: %v", err)
			return // unknown error
		}
	}
}
