package api

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/hound-search/hound/config"
	"github.com/hound-search/hound/index"
	"github.com/hound-search/hound/searcher"
)

var parseAsIntAndUintTests = map[string]struct {
	numResults string
	min int
	max	int
	def	int
	expected int
} {
	"parse error test case": {"not a parsable integer", 101, 1000, 5000, 5000},
	"less than min test case": {"100", 101, 1000, 5000, 101},
	"greater than max test case": {"1001", 101, 1000, 5000, 1000},
	"within limits test case": {"100", 0, 100, 5000, 100},
}

func TestParseAsIntAndUint(t *testing.T) {
	for name, td := range parseAsIntAndUintTests {
		t.Run(name, func(t *testing.T) {
			if got, expected := 
				parseAsUintValue(td.numResults, uint(td.min), uint(td.max), uint(td.def)), uint(td.expected); got != expected {
					t.Fatalf("parseAsUintValue - %s: returned %d; expected %d", name, got, expected)
				}

			if got, expected := 
				parseAsIntValue(td.numResults, td.min, td.max, td.def), td.expected; got != expected {
					t.Fatalf("parseAsIntValue - %s: returned %d; expected %d", name, got, expected)
				}
		})	
	}
}

func TestFilterByLabels(t *testing.T) {
	idx := map[string]*searcher.Searcher{
		"video-a":  {Repo: &config.Repo{Labels: []string{"video"}}},
		"access-a": {Repo: &config.Repo{Labels: []string{"access", "java"}}},
		"both":     {Repo: &config.Repo{Labels: []string{"video", "access"}}},
		"none":     {Repo: &config.Repo{}},
	}
	all := []string{"access-a", "both", "none", "video-a"}

	tests := map[string]struct {
		repos    []string
		labels   string
		expected []string
	}{
		"empty labels keeps all":     {all, "", all},
		"blank labels keeps all":     {all, " , ", all},
		"single label":               {all, "video", []string{"both", "video-a"}},
		"labels are a union":         {all, "video,java", []string{"access-a", "both", "video-a"}},
		"labels are trimmed":         {all, " access ", []string{"access-a", "both"}},
		"intersects with repo list":  {[]string{"access-a", "video-a"}, "video", []string{"video-a"}},
		"unknown label matches none": {all, "nope", nil},
	}

	for name, td := range tests {
		t.Run(name, func(t *testing.T) {
			got := filterByLabels(td.repos, td.labels, idx)
			sort.Strings(got)
			if !reflect.DeepEqual(got, td.expected) {
				t.Fatalf("filterByLabels(%v, %q) = %v; expected %v", td.repos, td.labels, got, td.expected)
			}
		})
	}
}

func TestUnknownLabels(t *testing.T) {
	idx := map[string]*searcher.Searcher{
		"video-a":  {Repo: &config.Repo{Labels: []string{"video"}}},
		"access-a": {Repo: &config.Repo{Labels: []string{"access"}}},
		"none":     {Repo: &config.Repo{}},
	}
	known := []string{"access", "video"}

	tests := map[string]struct {
		labels  string
		unknown []string
	}{
		"no labels":             {"", nil},
		"known labels":          {"video, access", nil},
		"misspelt label":        {"videos", []string{"videos"}},
		"one unknown in a list": {"video,bogus", []string{"bogus"}},
		"labels are case-exact": {"Video", []string{"Video"}},
	}

	for name, td := range tests {
		t.Run(name, func(t *testing.T) {
			unknown, gotKnown := unknownLabels(td.labels, idx)
			if !reflect.DeepEqual(unknown, td.unknown) || !reflect.DeepEqual(gotKnown, known) {
				t.Fatalf("unknownLabels(%q) = %v, %v; expected %v, %v", td.labels, unknown, gotKnown, td.unknown, known)
			}
		})
	}
}

func TestRepoResultJSON(t *testing.T) {
	resp := &index.SearchResponse{Matches: []*index.FileMatch{}, FilesWithMatch: 1, Revision: "abc"}

	tests := map[string]struct {
		repo     *config.Repo
		expected string
	}{
		"labelled repo":   {&config.Repo{Labels: []string{"video"}}, `["video"]`},
		"unlabelled repo": {&config.Repo{}, `[]`},
	}

	for name, td := range tests {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(&repoResult{resp, labelsOf(&searcher.Searcher{Repo: td.repo})})
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"Matches", "FilesWithMatch", "Revision"} {
				if _, ok := got[key]; !ok {
					t.Fatalf("%s missing from %s", key, raw)
				}
			}
			if string(got["Labels"]) != td.expected {
				t.Fatalf("Labels = %s; expected %s", got["Labels"], td.expected)
			}
		})
	}
}
