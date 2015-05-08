package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestRoundtripRequest(t *testing.T) {
	r, _ := http.NewRequest("GET", "/foo", bytes.NewReader([]byte("MESSAGE")))

	marshalled, err := json.Marshal(RequestJSONMarshaller{r})
	if err != nil {
		t.Fatalf("Error marshalling: %v", err)
	}

	r = &http.Request{}

	err = json.Unmarshal(marshalled, &RequestJSONMarshaller{r})
	if err != nil {
		t.Fatalf("Error unmarshalling: %v", err)
	}

	if r.URL.Path != "/foo" {
		t.Errorf("r.URL.Path != /foo (== %q)", r.URL.Path)
	}

}
