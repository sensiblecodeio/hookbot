package github

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"

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

	// "wss://hookbot.scraperwiki.com/unsafe/sub/github.com/org/scraperwiki"
	target := c.String("monitor-url")

	u, err := url.Parse(target)
	if err != nil {
		log.Fatalf("Failed to parse %q as URL: %v", target, u)
	}

	finish := make(chan struct{})

	header := MustParseHeaders(c)
	origin := c.String("origin")
	if origin == "samehost" {
		origin = "//" + u.Host
	}

	header.Add("Origin", origin)
	header.Add("X-Hookbot-Unsafe-Is-Ok", "I understand the security implications")

	messages, errors, err := listen.Watch(target, header, finish)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	for m := range messages {
		Route(m)
	}

	var sawError bool
	for err := range errors {
		log.Printf("Encountered error during message parsing: %v", err)
		sawError = true
	}
	if sawError {
		log.Fatalln("Errors encountered.")
	}
}

func Route(message listen.Message) {
	log.Println("Got message")
}
