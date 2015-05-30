package hookbot

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Topic string
	Body  []byte
	Done  chan struct{} // signalled when messages have been strobed
}

type Listener struct {
	Topic string
	c     chan Message
	ready chan struct{} // closed when c is subscribed
}

type Hookbot struct {
	key string

	wg       *sync.WaitGroup
	shutdown chan struct{}

	http.Handler

	message                  chan Message
	addListener, delListener chan Listener

	routers []Router

	listeners, publish, dropP, sends, dropS int64
}

func New(key string) *Hookbot {
	h := &Hookbot{
		key: key,

		wg:       &sync.WaitGroup{},
		shutdown: make(chan struct{}),

		message:     make(chan Message, 1),
		addListener: make(chan Listener, 1),
		delListener: make(chan Listener, 1),
	}

	sub := WebsocketHandlerFunc(h.ServeSubscribe)
	pub := http.HandlerFunc(h.ServePublish)

	mux := http.NewServeMux()
	mux.Handle("/sub/", h.KeyChecker(sub))
	mux.Handle("/pub/", h.KeyChecker(pub))

	mux.Handle("/unsafe/sub/", RequireUnsafeHeader(h.KeyChecker(sub)))

	mux.Handle("/unsafe/pub/", http.HandlerFunc(h.ServePublish))

	h.Handler = mux

	h.wg.Add(1)
	go h.Loop()

	h.wg.Add(1)
	go h.ShowStatus(time.Minute)

	return h
}

func (h *Hookbot) ShowStatus(period time.Duration) {
	defer h.wg.Done()
	ticker := time.NewTicker(period)
	var ll, lp, ls, ldP, ldS int64

	for {
		select {
		case <-ticker.C:
			l := atomic.LoadInt64(&h.listeners)
			p := atomic.LoadInt64(&h.publish)
			s := atomic.LoadInt64(&h.sends)
			dP := atomic.LoadInt64(&h.dropP)
			dS := atomic.LoadInt64(&h.dropS)

			log.Printf("Listeners %5d [%+5d] pub %5d [%+5d] (d %5d [%+5d])"+
				" send %8d [%+7d] (d %5d [%+5d])",
				l, l-ll, p, p-lp, dP, dP-ldP, s, s-ls, dS, dS-ldS)

			ll, lp, ls, ldP, ldS = l, p, s, dP, dS
		case <-h.shutdown:
			return
		}
	}
}

var timeout = 1 * time.Second

func TimeoutSend(wg *sync.WaitGroup, c chan<- Message, m Message) {
	defer wg.Done()

	select {
	case c <- m:
	case <-time.After(timeout):
	}
}

// Shut down main loop and wait for all in-flight messages to send or timeout
func (h *Hookbot) Shutdown() {
	close(h.shutdown)
	h.wg.Wait()
}

// Returns "true" if fullTopic ends with `?recursive` and returns topic name
// without `?recursive` suffix.
func recursive(fullTopic string) (topic string, isRecursive bool) {
	if strings.HasSuffix(fullTopic, "?recursive") {
		return fullTopic[:len(fullTopic)-len("?recursive")], true
	}
	return fullTopic, false
}

// Manage fanout from h.message onto listeners
func (h *Hookbot) Loop() {
	defer h.wg.Done()

	listeners := map[string]map[Listener]struct{}{}

	interested := func(topic string) map[Listener]struct{} {

		ls := map[Listener]struct{}{}

		for l := range listeners[topic] {
			ls[l] = struct{}{}
		}

		for fullCandidateTopic, candidateLs := range listeners {
			candidateTopic, isRec := recursive(fullCandidateTopic)
			if !isRec {
				continue
			}

			if !strings.HasPrefix(topic, candidateTopic) {
				continue
			}
			for l := range candidateLs {
				ls[l] = struct{}{}
			}
		}
		return ls
	}

	for {
		select {
		case m := <-h.message:

			for listener := range interested(m.Topic) {
				h.wg.Add(1)
				go TimeoutSend(h.wg, listener.c, m)
			}

			if m.Done != nil {
				close(m.Done)
			}

		case l := <-h.addListener:
			atomic.AddInt64(&h.listeners, 1)

			if _, ok := listeners[l.Topic]; !ok {
				listeners[l.Topic] = map[Listener]struct{}{}
			}
			listeners[l.Topic][l] = struct{}{}
			close(l.ready)

		case l := <-h.delListener:
			atomic.AddInt64(&h.listeners, -1)

			delete(listeners[l.Topic], l)
			if len(listeners[l.Topic]) == 0 {
				delete(listeners, l.Topic)
			}

		case <-h.shutdown:
			return
		}
	}
}

func (h *Hookbot) Add(topic string) Listener {
	ready := make(chan struct{})
	l := Listener{
		Topic: topic,

		c:     make(chan Message, 1),
		ready: ready,
	}
	h.addListener <- l
	<-ready
	return l
}

// Process messages for one router (one goroutine per topic)
func (h *Hookbot) AddRouter(r Router) {
	for _, topic := range r.Topics() {
		h.wg.Add(1)
		go func() {
			defer h.wg.Done()

			l := h.Add(topic)
			for m := range l.c {
				r.Route(m, h.Publish)
			}
		}()
	}
}

func (h *Hookbot) Del(l Listener) {
	h.delListener <- l
}

// The topic is everything after the "/pub/" or "/sub/"
// Do not capture the "/unsafe". See note in `Topic()`.
var TopicRE *regexp.Regexp = regexp.MustCompile("^(?:/unsafe)?/[^/]+/(.*)$")

func Topic(r *http.Request) string {
	m := TopicRE.FindStringSubmatch(r.URL.Path)
	if m == nil {
		return ""
	}
	topic := m[1]
	if IsUnsafeRequest(r) {

		return "/unsafe/" + topic
	}
	return topic
}

func (h *Hookbot) ServePublish(w http.ResponseWriter, r *http.Request) {

	done := make(chan struct{})

	topic := Topic(r)

	var (
		body []byte
		err  error
	)

	body, err = ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("Error in ServePublish reading body:", err)
		http.Error(w, "500 Internal Server Error",
			http.StatusInternalServerError)
		return
	}

	extraMetadata := r.URL.Query()["extra-metadata"]
	if len(extraMetadata) > 0 {
		switch extraMetadata[0] {
		case "github":

			body, err = json.Marshal(map[string]interface{}{
				"Signature": r.Header.Get("X-Hub-Signature"),
				"Event":     r.Header.Get("X-GitHub-Event"),
				"Delivery":  r.Header.Get("X-GitHub-Delivery"),
				"Payload":   body,
			})

			if err != nil {
				log.Println("Error in ServePublish serializing payload:", err)
				http.Error(w, "500 Internal Server Error",
					http.StatusInternalServerError)
			}

		default:
			http.Error(w, "400 Bad Request (bad ?extra-metadata=)",
				http.StatusBadRequest)
			return
		}
	}

	log.Printf("Publish %q", topic)

	h.Publish(Message{Topic: topic, Body: body, Done: done})

	fmt.Fprintln(w, "OK")
}

// Blocks until message has been published.
func (h *Hookbot) Publish(m Message) {
	done := make(chan struct{})
	m.Done = done
	h.message <- m
	<-done
}

func (h *Hookbot) ServeSubscribe(conn *websocket.Conn, r *http.Request) {

	topic := Topic(r)

	listener := h.Add(topic)
	defer h.Del(listener)

	closed := make(chan struct{})

	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				conn.Close()
				return
			}
		}
	}()

	var message Message

	for {
		select {
		case message = <-listener.c:
		case <-closed:
			log.Printf("Client disconnected")
			return
		}

		conn.SetWriteDeadline(time.Now().Add(90 * time.Second))
		err := conn.WriteMessage(websocket.BinaryMessage, message.Body)
		switch {
		case err == io.EOF || IsConnectionClose(err):
			return
		case err != nil:
			log.Printf("Error in conn.WriteMessage: %v", err)
			return
		}
	}
}

func IsConnectionClose(err error) bool {
	if err == nil {
		return false
	}
	str := err.Error()
	switch {
	case strings.HasSuffix(str, "broken pipe"):
		return true
	case strings.HasSuffix(str, "connection reset by peer"):
		return true
	case strings.HasSuffix(str, "use of closed network connection"):
		return true
	}
	return false
}
