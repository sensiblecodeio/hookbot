package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
)

func main() {
	var (
		addr = flag.String("addr", ":8080", "address to listen on")
		key  = os.Getenv("HOOKBOT_KEY")
	)
	flag.Parse()

	if key == "" {
		log.Fatalln("Error: HOOKBOT_KEY not set")
	}

	handler := NewHookbot(key)

	log.Println("Listening on", *addr)
	err := http.ListenAndServe(*addr, handler)
	if err != nil {
		log.Fatal(err)
	}
}

type Hookbot struct {
	key string

	*http.ServeMux

	message                  chan []byte
	addListener, delListener chan chan []byte
}

func NewHookbot(key string) *Hookbot {
	h := &Hookbot{
		key: key,

		ServeMux: http.NewServeMux(),

		message:     make(chan []byte, 10),
		addListener: make(chan chan []byte, 10),
		delListener: make(chan chan []byte, 10),
	}

	h.HandleFunc("/watch/", h.ServeWatch)
	h.HandleFunc("/notify/", h.ServeNotify)

	go h.Loop()

	return h
}

// Manage fanout from h.message onto listeners
func (h *Hookbot) Loop() {
	listeners := map[chan []byte]struct{}{}
	for {
		select {
		case m := <-h.message:
			for listener := range listeners {
				listener <- m
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

func (h *Hookbot) BadKey(route string, w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == route+h.key {
		return false
	}
	http.NotFound(w, r)
	return true
}

func (h *Hookbot) ServeNotify(w http.ResponseWriter, r *http.Request) {
	if h.BadKey("/notify/", w, r) {
		return
	}
	message, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error serving %v: %v", r.URL, err)
		return
	}
	h.message <- message
}

var upgrader = websocket.Upgrader{}

func (h *Hookbot) ServeWatch(w http.ResponseWriter, r *http.Request) {
	if h.BadKey("/watch/", w, r) {
		return
	}

	c := h.Add()
	defer h.Del(c)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error upgrading connection:", err)
		return
	}

	closed := make(chan struct{})

	go func() {
		defer close(closed)

		// Read loop is required
		for {
			if _, _, err := conn.NextReader(); err != nil {
				conn.Close()
				break
			}
		}
	}()

	for {
		var m []byte
		select {
		case m = <-c:
		case <-closed:
			log.Println("Client disconnected")
			return
		}
		err := conn.WriteMessage(websocket.TextMessage, m)
		if err != nil {
			log.Println("Error in writemessage:", err)
			break
		}
	}

}
