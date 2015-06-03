package hookbot

import (
	"testing"
)

// Ensure that messages are delivered to the intended topics.
// Deliver two messages to two different topics and check they arrive.
func TestTopicsIndependent(t *testing.T) {

	var c1, c2 chan Message

	func() {
		hookbot := New(TEST_KEY)
		defer hookbot.Shutdown()

		msgsC1 := hookbot.Add("/unsafe/1")
		msgsC2 := hookbot.Add("/unsafe/2")

		c1, c2 = msgsC1.c, msgsC2.c

		hookbot.ServeHTTP(MakeRequest("POST", "/unsafe/pub/1", "MESSAGE 1"))
		hookbot.ServeHTTP(MakeRequest("POST", "/unsafe/pub/2", "MESSAGE 2"))
	}()

	checkDelivered := func(c chan Message, expected string) {
		select {
		case m := <-c:
			if string(m.Body) != expected {
				t.Errorf("m != %s (=%q)", expected, string(m.Body))
			}
		default:
			t.Fatalf("Message not delivered correctly: %q", expected)
		}
	}

	checkDelivered(c1, "MESSAGE 1")
	checkDelivered(c2, "MESSAGE 2")
}

// Ensure that messages are delivered to recursive listeners.
func TestTopicsRecursive(t *testing.T) {

	var c1, c2 chan Message

	func() {
		hookbot := New(TEST_KEY)
		defer hookbot.Shutdown()

		msgsC1 := hookbot.Add("/unsafe/foo/?recursive")
		msgsC2 := hookbot.Add("/unsafe/foo/bar")

		c1, c2 = msgsC1.c, msgsC2.c

		w, r := MakeRequest("POST", "/unsafe/pub/foo/bar", "MESSAGE")
		hookbot.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Errorf("Fail: %v", w.Code)
		}
	}()

	checkDelivered := func(c chan Message, expected string) bool {
		select {
		case m := <-c:
			if string(m.Body) != expected {
				t.Errorf("m != %s (=%q)", expected, string(m.Body))
			}
		default:
			return false
		}
		return true
	}

	if !checkDelivered(c1, "MESSAGE") {
		t.Errorf("Message not delivered")
	}
	if !checkDelivered(c2, "MESSAGE") {
		t.Errorf("Message not delivered")
	}
}

// Ensure that messages are not delivered recursively if ?recursive is omitted
func TestTopicsNotRecursive(t *testing.T) {

	var c1, c2 chan Message

	func() {
		hookbot := New(TEST_KEY)
		defer hookbot.Shutdown()

		msgsC1 := hookbot.Add("/unsafe/foo/")
		msgsC2 := hookbot.Add("/unsafe/foo/bar")

		c1, c2 = msgsC1.c, msgsC2.c

		w, r := MakeRequest("POST", "/unsafe/pub/foo/bar", "MESSAGE")
		hookbot.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Errorf("Fail: %v", w.Code)
		}
	}()

	checkDelivered := func(c chan Message, expected string) bool {
		select {
		case m := <-c:
			if string(m.Body) != expected {
				t.Errorf("m != %s (=%q)", expected, string(m.Body))
			}
		default:
			return false
		}
		return true
	}

	// c2 should get the message since it listened directly to the target topic
	if !checkDelivered(c2, "MESSAGE") {
		t.Errorf("Message not delivered to c2")
	}

	select {
	case <-c1:
		t.Errorf("Message erroneously delivered to c1")
	default:
	}
}
