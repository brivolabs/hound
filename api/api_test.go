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
			got := filterByLabels(td.repos, parseLabels(td.labels), idx)
			sort.Strings(got)
			if !reflect.DeepEqual(got, td.expected) {
				t.Fatalf("filterByLabels(%v, %q) = %v; expected %v", td.repos, td.labels, got, td.expected)
			}
		})
	}
}

func TestSelectRepos(t *testing.T) {
	idx := map[string]*searcher.Searcher{
		"video-a":  {Repo: &config.Repo{Labels: []string{"video"}}},
		"access-a": {Repo: &config.Repo{Labels: []string{"access"}}},
		"none":     {Repo: &config.Repo{}},
	}
	inUse := labelsInUse(idx)

	tests := map[string]struct {
		repos    string
		labels   string
		expected []string
		err      string
	}{
		"no repos no labels":         {"", "", nil, ""},
		"repos without labels":       {"access-a,none", "", []string{"access-a", "none"}, ""},
		"labels alone select all":    {"", "video, access", []string{"access-a", "video-a"}, ""},
		"comma-only labels are none": {"", " , ", nil, ""},
		"labels narrow named repos":  {"access-a,video-a", "video", []string{"video-a"}, ""},
		"union keeps partial match":  {"access-a", "access,video", []string{"access-a"}, ""},
		"misspelt label": {"", "videos", nil,
			"unknown label(s): videos (labels in use: access, video)"},
		"unknown labels deduped and sorted": {"", "video,zz,bogus,zz", nil,
			"unknown label(s): bogus, zz (labels in use: access, video)"},
		"labels are case-exact": {"", "Video", nil,
			"unknown label(s): Video (labels in use: access, video)"},
		"label not on named repos": {"access-a", "video", nil,
			"none of the selected repos has label(s): video"},
	}

	for name, td := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := selectRepos(td.repos, td.labels, idx, inUse)
			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, td.expected) || gotErr != td.err {
				t.Fatalf("selectRepos(%q, %q) = %v, %q; expected %v, %q", td.repos, td.labels, got, gotErr, td.expected, td.err)
			}
		})
	}
}

func TestSelectReposWithoutLabelsInUse(t *testing.T) {
	idx := map[string]*searcher.Searcher{"none": {Repo: &config.Repo{}}}
	_, err := selectRepos("", "video", idx, labelsInUse(idx))
	if err == nil || err.Error() != "unknown label(s): video (no repo has labels)" {
		t.Fatalf("unexpected error: %v", err)
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
