package listen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

type Message struct {
	*http.Request
}

func (r Message) MarshalJSON() ([]byte, error) {

	asJSON := func(v interface{}) ([]byte, error) {
		marshalled, err := json.Marshal(v)
		if err != nil {
			err = fmt.Errorf("serialize: error marshalling %T %v: %v", v, v, err)
			return nil, err
		}
		return marshalled, nil
	}

	var buf bytes.Buffer

	fmt.Fprint(&buf, "{") // open whole document

	// Do not forward the authorization header to listeners.
	r.Header.Del("Authorization")

	header, err := asJSON(r.Header)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(&buf, `"URL": "%s", `, r.URL.Path)
	fmt.Fprintf(&buf, `"RemoteAddr": "%s", `, r.RemoteAddr)
	fmt.Fprintf(&buf, `"Header": %s, `, header)

	fmt.Fprint(&buf, `"Body": `) // follows

	if r.Header.Get("Content-Type") == "application/json" {
		//
		_, err := buf.ReadFrom(r.Body)
		if err != nil {
			err = fmt.Errorf("serialize: error reading request body: %v", err)
			return nil, err
		}
	} else {
		bs, err := ioutil.ReadAll(r.Body)
		if err != nil {
			err = fmt.Errorf("serialize: error reading request body: %v", err)
			return nil, err
		}
		bs, err = asJSON(string(bs))
		if err != nil {
			err = fmt.Errorf("serialize: error marshalling: %v", err)
			return nil, err
		}
		fmt.Fprintf(&buf, "%s", bs)
	}
	r.Body.Close()

	fmt.Fprint(&buf, "}") // close whole document

	return buf.Bytes(), nil
}

func (r *Message) UnmarshalJSON(data []byte) error {

	if r.Request == nil {
		r.Request = &http.Request{}
	}

	type DecodeBuf struct {
		URL        string
		RemoteAddr string
		Header     http.Header
		Body       interface{}
	}

	d := DecodeBuf{}
	err := json.Unmarshal(data, &d)
	if err != nil {
		return err
	}

	body, err := json.Marshal(d.Body)
	if err != nil {
		return err
	}

	r.URL, err = url.Parse(d.URL)
	if err != nil {
		return fmt.Errorf("error parsing URL %q: %v", d.URL, err)
	}
	r.RemoteAddr = d.RemoteAddr
	r.Header = d.Header
	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

	return nil
}
