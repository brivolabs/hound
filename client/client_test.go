package client

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func searchAgainst(t *testing.T, body string) (*Response, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	var res Response
	cfg := &Config{Host: strings.TrimPrefix(srv.URL, "http://")}
	err := Search(&res, cfg, "foo", "*", "", "", 0, false, false)
	return &res, err
}

func TestSearchReadsLabels(t *testing.T) {
	res, err := searchAgainst(t, `{"Results":{"video-a":{"Matches":[],"FilesWithMatch":1,"Revision":"abc","Labels":["video"]}}}`)
	if err != nil {
		t.Fatal(err)
	}
	got := res.Results["video-a"]
	if got == nil || got.Revision != "abc" || !reflect.DeepEqual(got.Labels, []string{"video"}) {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestSearchReturnsAPIError(t *testing.T) {
	_, err := searchAgainst(t, `{"Error":"unknown label(s): videos (labels in use: video)"}`)
	if err == nil || err.Error() != "unknown label(s): videos (labels in use: video)" {
		t.Fatalf("expected the API error, got %v", err)
	}
}
