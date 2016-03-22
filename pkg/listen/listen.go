package listen

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type ErrConnectionFail struct {
	resp *http.Response
	err  error
}

func (e *ErrConnectionFail) Error() string {
	err := e.err
	if e.resp != nil {
		err = fmt.Errorf("%v: response: %v", err, e.resp.Status)
	}
	return fmt.Sprintf("failed: %v", err)
}

func Watch(
	target string, header http.Header, finish <-chan struct{},
) (<-chan []byte, <-chan error, error) {

	u, err := url.Parse(target)
	if err != nil {
		return nil, nil, err
	}

	if strings.HasPrefix(u.Path, "/xub/") {
		u.Path = "/sub/" + strings.TrimPrefix(u.Path, "/xub/")
		switch u.Scheme {
		case "http":
			u.Scheme = "ws"
		case "https":
			u.Scheme = "wss"
		}
	}

	if u.User != nil {
		oldHeader := header

		// Avoid modifying caller's headers
		header = http.Header{}
		for k, v := range oldHeader {
			header[k] = v
		}

		userPassBytes := []byte(u.User.String() + ":")
		token := base64.StdEncoding.EncodeToString(userPassBytes)
		header.Set("Authorization", fmt.Sprintf("Basic %v", token))
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
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	messages := make(chan []byte, 1)
	errors := make(chan error, 1)
	readerDone := make(chan struct{})

	// Writer goroutine
	go func() {
		defer conn.Close()

		for {
			select {
			case <-time.After(15*time.Second + Jitter(5)):
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
		defer close(messages)

		for {
			_, r, err := conn.NextReader()
			if err != nil {
				select {
				case <-finish:
					// We've been requested to finish, ignore the error.
					return
				default:
				}

				errors <- err
				log.Printf("Error in NextReader(): %v", err)
				return
			}

			m, err := ioutil.ReadAll(r)
			if err != nil {
				select {
				case <-finish:
					// We've been requested to finish, ignore the error.
					return
				default:
				}

				errors <- err
				log.Printf("Error in ReadAll(): %v", err)
				return
			}

			select {
			case messages <- m:
			case <-finish:
				return
			}

		}
	}()

	return messages, errors, nil
}

// This function is like Watch() except if the transport fails, it is
// automatically retried.
func RetryingWatch(
	target string, header http.Header, finish <-chan struct{},
) (<-chan []byte, <-chan error) {

	outm := make(chan []byte)
	oute := make(chan error)

	go func() {
		defer close(outm)
		defer close(oute)

		var wg sync.WaitGroup
		defer wg.Wait()

		for {
			ms, errs, err := Watch(target, header, finish)
			if err != nil {
				oute <- err
				goto retry
			}

			log.Printf("Connected to %q", target)

			wg.Add(1)
			go func() {
				defer wg.Done()
				for m := range ms {
					outm <- m
				}
			}()

			for err := range errs {
				oute <- err
			}

			select {
			case <-finish:
				return
			default:
			}

		retry:
			log.Printf("Connection failed. Retrying in 5 seconds.")
			time.Sleep(5*time.Second + Jitter(1))
		}
	}()

	return outm, oute
}

// Return a random duration from -1s to +1s
func Jitter(mul int) time.Duration {
	m := time.Duration(mul)
	return time.Duration(rand.Intn(int(m*2*time.Second))) - m*1*time.Second
}
