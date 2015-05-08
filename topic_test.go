package main

import (
	"encoding/json"
	"testing"
)

func getBody(m []byte) (string, error) {
	type Buf struct {
		Body string
	}

	buf := Buf{}
	err := json.Unmarshal(m, &buf)
	if err != nil {
		return "", err
	}

	return buf.Body, nil
}

// Ensure that messages are delivered to the intended topics.
// Deliver two messages to two different topics and check they arrive.
func TestTopicsIndependent(t *testing.T) {

	var c1, c2 chan []byte

	func() {
		hookbot := NewHookbot(TEST_KEY, TEST_GITHUB_SECRET)
		defer hookbot.Shutdown()

		msgsC1 := hookbot.Add("/unsafe/1")
		msgsC2 := hookbot.Add("/unsafe/2")
		defer hookbot.Del(msgsC1)
		defer hookbot.Del(msgsC2)

		c1, c2 = msgsC1.c, msgsC2.c

		hookbot.ServeHTTP(MakeRequest("POST", "/unsafe/pub/1", "MESSAGE 1"))
		hookbot.ServeHTTP(MakeRequest("POST", "/unsafe/pub/2", "MESSAGE 2"))
	}()

	checkDelivered := func(c chan []byte, expected string) {
		select {
		case m := <-c:
			body, err := getBody(m)
			if err != nil {
				t.Fatalf("Failed to decode body: %v (%q)", err, m)
			}
			if body != expected {
				t.Errorf("m != %s (=%q)", expected, body)
			}
		default:
			t.Fatalf("Message not delivered correctly: %q", expected)
		}
	}

	checkDelivered(c1, "MESSAGE 1")
	checkDelivered(c2, "MESSAGE 2")
}
