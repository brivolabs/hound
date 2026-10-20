package api

import (
	"reflect"
	"sort"
	"testing"

	"github.com/hound-search/hound/config"
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
