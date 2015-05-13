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
	"time"

	"github.com/codegangsta/cli"
	"github.com/scraperwiki/hookbot/listen"
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

func MustParseHeaders(c *cli.Context) http.Header {
	headers := http.Header{}

	for _, h := range c.StringSlice("header") {
		key, value := MustParseHeader(h)
		headers.Set(key, value)
	}

	return headers
}

func ActionRoute(c *cli.Context) {

	target := c.String("monitor-url")

	u, err := url.Parse(target)
	if err != nil {
		log.Fatalf("Failed to parse %q as URL: %v", target, u)
	}

	header := MustParseHeaders(c)
	origin := c.String("origin")
	if origin == "samehost" {
		origin = "//" + u.Host
	}

	header.Add("Origin", origin)
	header.Add("X-Hookbot-Unsafe-Is-Ok", "I understand the security implications")

	key := c.GlobalString("key")
	github_secret := c.GlobalString("github-secret")

	for {
		Connection(key, github_secret, u, header)

		log.Println("Router failed; waiting 5 seconds and retrying…")
		time.Sleep(5 * time.Second)
	}
}

func Connection(key, github_secret string, u *url.URL, header http.Header) {
	finish := make(chan struct{})

	messages, errors, err := listen.Watch(u.String(), header, finish)
	if err != nil {
		log.Printf("Failed to connect: %v", err)
		return
	}

	log.Println("Connection to hookbot instance established")

	outbound := make(chan listen.Message, 1)

	go func() {
		for m := range outbound {
			outURL := m.URL

			token := Sha1HMAC(key, outURL.Path)

			outURL.Scheme = "https"
			outURL.Host = u.Host

			out, err := http.NewRequest("POST", outURL.String(), m.Body)
			if err != nil {
				log.Printf("Failed to construct outbound req: %v", err)
				continue
			}
			out.SetBasicAuth(token, "")

			out.Header.Set("Content-Type", "application/hookbot+raw")

			resp, err := http.DefaultClient.Do(out)
			if err != nil {
				log.Printf("Failed to transmit: %v", err)
				continue
			}
			log.Printf("Transmit: %v %v", resp.StatusCode, outURL.String())
		}
	}()

	for m := range messages {
		if !IsValidGithubSignature(github_secret, m.Request) {
			log.Printf("Reject github signature")
			continue
		}

		routed, ok := Route(m)
		if !ok {
			continue
		}

		outbound <- routed
	}
	close(outbound)

	for err := range errors {
		log.Printf("Encountered error during message parsing: %v", err)
	}
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

func Route(message listen.Message) (listen.Message, bool) {

	if !strings.HasPrefix(message.URL.Path, "/unsafe/") {
		log.Printf("Message URL does not begin with /unsafe/, ignoring %q",
			message.URL.Path)
		return listen.Message{}, false
	}

	payload, err := message.Payload()
	if err != nil {
		log.Printf("Failed to obtain payload: %v", err)
		return listen.Message{}, false
	}

	var v Event
	v.Type = message.Header.Get("X-Github-Event")

	err = json.Unmarshal(payload, &v)
	if err != nil {
		log.Printf("Route: error in json.Unmarshal: %v", err)
		return listen.Message{}, false
	}

	if v.Repository == nil || v.Repository.FullName == "" {
		log.Printf("Could not identify repository for event %v", v.Type)
		return listen.Message{}, false
	}

	repo := v.Repository.FullName
	branch := v.Branch()

	urlFmt := "/pub/github.com/repo/%s/push/branch/%s"
	message.URL.Path = fmt.Sprintf(urlFmt, repo, branch)

	type Update struct {
		Repo, Branch, SHA, Who string
	}

	msgBytes, err := json.Marshal(&Update{
		repo, branch, v.After, v.Pusher.Name,
	})
	if err != nil {
		log.Printf("Failed to marshal Update: %v", err)
		return listen.Message{}, false
	}

	message.Body = ioutil.NopCloser(bytes.NewBuffer(msgBytes))

	return message, true
}
