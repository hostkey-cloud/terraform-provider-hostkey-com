package provider

import (
	"encoding/json"
	"testing"
)

func TestServerLocation(t *testing.T) {
	cases := map[string]string{
		`{"location":"NL"}`:            "NL",
		`{"location_name":" RU "}`:     "RU",
		`{"location":{"name":"FI"}}`:   "FI",
		`{"preset":{"location":"DE"}}`: "",
		`null`:                         "",
		``:                             "",
		`not json`:                     "",
	}
	for in, want := range cases {
		if got := serverLocation(json.RawMessage(in)); got != want {
			t.Errorf("serverLocation(%q) = %q, want %q", in, got, want)
		}
	}
}
