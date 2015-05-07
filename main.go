package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/codegangsta/cli"
	"golang.org/x/net/websocket"
)

func main() {
	app := cli.NewApp()
	app.Usage = "turn webhooks into websockets"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "bind, b",
			Value: ":8080",
			Usage: "address to listen on",
		},
	}

	app.Action = ActionMain

	app.RunAndExitOnError()
}

func ActionMain(c *cli.Context) {
	var (
		key           = os.Getenv("HOOKBOT_KEY")
		github_secret = os.Getenv("HOOKBOT_GITHUB_SECRET")
	)

	if key == "" || github_secret == "" {
		log.Fatalln("Error: HOOKBOT_KEY or HOOKBOT_GITHUB_SECRET not set")
	}

	hookbot := NewHookbot(key, github_secret)
	http.Handle("/", hookbot)
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "OK")
	})

	log.Println("Listening on", c.String("bind"))
	err := http.ListenAndServe(c.String("bind"), nil)
	if err != nil {
		log.Fatal(err)
	}
}

type Message struct {
	Topic string
	Body  []byte
}

type Listener struct {
	Topic string
	c     chan []byte
}

type Hookbot struct {
	key, github_secret string

	http.Handler

	message                  chan Message
	addListener, delListener chan Listener
}

func NewHookbot(key, github_secret string) *Hookbot {
	h := &Hookbot{
		key: key, github_secret: github_secret,

		message:     make(chan Message, 1),
		addListener: make(chan Listener, 1),
		delListener: make(chan Listener, 1),
	}

	mux := http.NewServeMux()
	mux.Handle("/sub/", websocket.Handler(h.ServeSubscribe))
	mux.HandleFunc("/pub/", h.ServePublish)

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
	listeners := map[Listener]struct{}{}
	for {
		select {
		case m := <-h.message:
			for listener := range listeners {
				if listener.Topic == m.Topic {
					go TimeoutSend(listener.c, m.Body)
				}
			}

		case l := <-h.addListener:
			listeners[l] = struct{}{}
		case l := <-h.delListener:
			delete(listeners, l)
		}
	}
}

func (h *Hookbot) Add(topic string) Listener {
	l := Listener{Topic: topic, c: make(chan []byte)}
	h.addListener <- l
	return l
}

func (h *Hookbot) Del(l Listener) {
	h.delListener <- l
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

// The topic is everything after the "/pub/" or "/sub/"
var TopicRE *regexp.Regexp = regexp.MustCompile("/[^/]+/(.*)")

func Topic(url *url.URL) string {
	m := TopicRE.FindStringSubmatch(url.Path)
	if m == nil {
		return ""
	}
	return m[1]
}

func (h *Hookbot) ServePublish(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)

	if err != nil {
		log.Printf("Error serving %v: %v", r.URL, err)
		return
	}

	topic := Topic(r.URL)
	h.message <- Message{Topic: topic, Body: body}
}

func (h *Hookbot) ServeSubscribe(conn *websocket.Conn) {

	topic := Topic(conn.Request().URL)

	listener := h.Add(topic)
	defer h.Del(listener)

	closed := make(chan struct{})

	go func() {
		defer close(closed)
		_, _ = io.Copy(ioutil.Discard, conn)
	}()

	var message []byte

	for {
		select {
		case message = <-listener.c:
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
