package listen

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

type ErrConnectionFail struct {
	resp *http.Response
	err  error
}

func (e *ErrConnectionFail) Error() string {
	var status *int
	if e.resp != nil {
		status = &e.resp.StatusCode
	}
	return fmt.Sprintf("connection failure (status %v): %v", status, e.err)
}

func Watch(
	target string, header http.Header, finish <-chan struct{},
) (<-chan Message, <-chan error, error) {

	u, err := url.Parse(target)
	if err != nil {
		return nil, nil, err
	}

	if u.User != nil {
		userPassBytes := []byte(u.User.String() + ":")
		token := base64.StdEncoding.EncodeToString(userPassBytes)
		header.Add("Authorization", fmt.Sprintf("Basic %v", token))
		u.User = nil
	}

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), header)
	if err != nil {
		return nil, nil, &ErrConnectionFail{resp, err}
	}

	const (
		pongWait = 40 * time.Second
	)

	const MiB = 1 << 20
	conn.SetReadLimit(1 * MiB)

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		log.Println("Pong")
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	messages := make(chan Message, 1)
	errors := make(chan error, 1)
	readerDone := make(chan struct{})

	// Writer goroutine
	go func() {
		defer close(messages)
		defer conn.Close()

		for {
			select {
			case <-time.After(20 * time.Second):
				log.Println("Ping")
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				err := conn.WriteMessage(websocket.PingMessage, []byte{})
				if err != nil {
					log.Printf("Error in WriteMessage: %v", err)
					return
				}
			case <-finish:
				return
			case <-readerDone:
				return
			}
		}
	}()

	// Reader goroutine
	go func() {
		defer close(readerDone)
		defer close(errors)

		for {
			_, r, err := conn.NextReader()
			if err != nil {
				errors <- err
				log.Printf("Error in NextReader(): %v", err)
				return
			}

			var m Message

			err = json.NewDecoder(r).Decode(&m)
			if err != nil {
				log.Printf("Error in json.NewDecoder(r).Decode(): %v", err)
				errors <- err
				return
			}

			messages <- m
		}
	}()

	return messages, errors, nil
}
