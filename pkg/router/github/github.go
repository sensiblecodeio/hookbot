package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/codegangsta/cli"

	"github.com/scraperwiki/hookbot/pkg/hookbot"
	"github.com/scraperwiki/hookbot/pkg/listen"
)

var RegexParseHeader = regexp.MustCompile("^\\s*([^\\:]+)\\s*:\\s*(.*)$")

func MustParseHeader(header string) (string, string) {
	if !RegexParseHeader.MatchString(header) {
		log.Fatalf("Unable to parse header: %v (re: %v)", header,
			RegexParseHeader.String())
		return "", ""
	}

	parts := RegexParseHeader.FindStringSubmatch(header)
	return parts[1], parts[2]
}

func MustParseHeaders(headerStrings []string) http.Header {
	headers := http.Header{}

	for _, h := range headerStrings {
		key, value := MustParseHeader(h)
		headers.Set(key, value)
	}

	return headers
}

func MustMakeHeader(
	target *url.URL, origin string, headerStrings []string,
) http.Header {

	header := MustParseHeaders(headerStrings)
	if origin == "samehost" {
		origin = "//" + target.Host
	}

	header.Add("Origin", origin)
	header.Add("X-Hookbot-Unsafe-Is-Ok",
		"I understand the security implications")

	return header
}

func ActionRoute(c *cli.Context) {

	target, err := url.Parse(c.String("monitor-url"))
	if err != nil {
		log.Fatalf("Failed to parse %q as URL: %v", c.String("monitor-url"), err)
	}

	origin := c.String("origin")

	header := MustMakeHeader(target, origin, c.StringSlice("header"))
	finish := make(chan struct{})

	messages, errors := listen.RetryingWatch(target.String(), header, finish)

	outbound := make(chan listen.Message, 1)

	send := func(endpoint string, payload []byte) {
		token := Sha1HMAC(c.GlobalString("key"), []byte(endpoint))

		outURL := fmt.Sprintf("https://%v@%v/pub/%v", token, target.Host, endpoint)

		body := ioutil.NopCloser(bytes.NewBuffer(payload))

		out, err := http.NewRequest("POST", outURL, body)
		if err != nil {
			log.Printf("Failed to construct outbound req: %v", err)
			return
		}
		out.SetBasicAuth(token, "")

		resp, err := http.DefaultClient.Do(out)
		if err != nil {
			log.Printf("Failed to transmit: %v", err)
			return
		}
		log.Printf("Transmit: %v %v", resp.StatusCode, outURL)
	}

	go func() {
		for err := range errors {
			log.Printf("Encountered error in Watch: %v", err)
		}
	}()

	for m := range messages {
		log.Printf("Receive message")
		if !IsValidGithubSignature(c.GlobalString("github-secret"), m) {
			log.Printf("Reject github signature")
			continue
		}

		Route(m, send)
	}
	close(outbound)
}

type Event struct {
	Type string

	Repository *Repository `json:"repository"`
	Pusher     *Pusher     `json:"pusher"`

	Ref    string `json:"ref"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func (e *Event) Branch() string {
	return strings.TrimPrefix(e.Ref, "refs/heads/")
}

type Pusher struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Repository struct {
	FullName string `json:"full_name"`
}

func Route(message []byte, send func(string, []byte)) {

	type GithubMessage struct {
		Event, Signature string
		Payload          []byte
	}

	var m GithubMessage

	err := json.Unmarshal(message, &m)
	if err != nil {
		log.Printf("Failed to unmarshal message in IsValidGithubSignature: %v",
			err)
		return
	}

	var event Event
	event.Type = m.Event

	err = json.Unmarshal(m.Payload, &event)
	if err != nil {
		log.Printf("Route: error in json.Unmarshal: %v", err)
		return
	}

	if event.Repository == nil || event.Repository.FullName == "" {
		log.Printf("Could not identify repository for event %v", event.Type)
		return
	}

	repo := event.Repository.FullName
	branch := event.Branch()

	who := "<unknown>"
	if event.Pusher != nil {
		who = event.Pusher.Name
	}

	msgBytes, err := json.Marshal(map[string]string{
		"Type":   event.Type,
		"Repo":   repo,
		"Branch": branch,
		"SHA":    event.After,
		"Who":    who,
	})
	if err != nil {
		log.Printf("Failed to marshal Update: %v", err)
		return
	}

	switch event.Type {
	case "push":
		topicFmt := "github.com/repo/%s/push/branch/%s"
		topic := fmt.Sprintf(topicFmt, repo, branch)
		send(topic, msgBytes)
	default:
		log.Printf("Unhandled event type: %v", event.Type)
		return
	}
}

type Router struct{}

func (r *Router) Name() string {
	return "github"
}

func (r *Router) Topics() []string {
	return []string{"/unsafe/github.com/?recursive"}
}

func (r *Router) Route(in hookbot.Message, publish func(hookbot.Message)) {

	log.Printf("route github: %q", in.Topic)

	type GithubMessage struct {
		Event, Signature string
		Payload          []byte
	}

	var m GithubMessage

	err := json.Unmarshal(in.Body, &m)
	if err != nil {
		log.Printf("Failed to unmarshal message in IsValidGithubSignature: %v",
			err)
		return
	}

	var event Event
	event.Type = m.Event

	err = json.Unmarshal(m.Payload, &event)
	if err != nil {
		log.Printf("Route: error in json.Unmarshal: %v", err)
		return
	}

	if event.Repository == nil || event.Repository.FullName == "" {
		log.Printf("Could not identify repository for event %v", event.Type)
		return
	}

	repo := event.Repository.FullName
	branch := event.Branch()

	who := "<unknown>"
	if event.Pusher != nil {
		who = event.Pusher.Name
	}

	msgBytes, err := json.Marshal(map[string]string{
		"Type":   event.Type,
		"Repo":   repo,
		"Branch": branch,
		"SHA":    event.After,
		"Who":    who,
	})
	if err != nil {
		log.Printf("Failed to marshal Update: %v", err)
		return
	}

	switch event.Type {
	case "push":
		topicFmt := "github.com/repo/%s/push/branch/%s"

		publish(hookbot.Message{
			Topic: fmt.Sprintf(topicFmt, repo, branch),
			Body:  msgBytes,
		})
	default:
		log.Printf("Unhandled event type: %v", event.Type)
		return
	}

}

func init() {
	hookbot.RegisterRouter(&Router{})
}
