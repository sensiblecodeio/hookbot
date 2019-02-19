package hookbot

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader websocket.Upgrader

func ConfigureServeHTTP(allowedOrigins []string) {

	if len(allowedOrigins) == 0 {
		upgrader = websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}
	} else {
		upgrader = websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				for _, o := range r.Header["Origin"] {
					for _, ao := range allowedOrigins {
						if o == ao {
							return true
						}
					}
				}
				return false
			},
		}
	}
}

type WebsocketHandlerFunc func(*websocket.Conn, *http.Request)

func (wrapped WebsocketHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade: %v", err)
		// Don't send any response here, Upgrade already does that on error.
		return
	}

	wrapped(conn, r)
}
