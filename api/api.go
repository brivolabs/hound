package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hound-search/hound/config"
	"github.com/hound-search/hound/index"
	"github.com/hound-search/hound/searcher"
)

const (
	defaultLinesOfContext uint = 2
	maxLinesOfContext     uint = 20
	maxLimit              int  = 100000
)

type Stats struct {
	FilesOpened int
	Duration    int
}

func writeJson(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Panicf("Failed to encode JSON: %v\n", err)
	}
}

func writeResp(w http.ResponseWriter, data interface{}) {
	writeJson(w, data, http.StatusOK)
}

func writeError(w http.ResponseWriter, err error, status int) {
	writeJson(w, map[string]string{
		"Error": err.Error(),
	}, status)
}

type searchResponse struct {
	repo string
	res  *index.SearchResponse
	err  error
}

// One repo's results as the API returns them. Labels is always present, even
// when empty, so a client can tell this server from one that doesn't support
// labels and would have ignored the filter.
type repoResult struct {
	*index.SearchResponse
	Labels []string
}

func labelsOf(srch *searcher.Searcher) []string {
	if srch.Repo.Labels == nil {
		return []string{}
	}
	return srch.Repo.Labels
}

/**
 * Searches all repos in parallel.
 */
func searchAll(
	query string,
	opts *index.SearchOptions,
	repos []string,
	idx map[string]*searcher.Searcher,
	filesOpened *int,
	duration *int,
) (map[string]*repoResult, error) {
	startedAt := time.Now()

	n := len(repos)

	// use a buffered channel to avoid routine leaks on errs.
	ch := make(chan *searchResponse, n)
	for _, repo := range repos {
		go func(repo string) {
			fms, err := idx[repo].Search(query, opts)
			ch <- &searchResponse{repo, fms, err}
		}(repo)
	}

	res := map[string]*repoResult{}
	for i := 0; i < n; i++ {
		r := <-ch
		if r.err != nil {
			return nil, r.err
		}

		if r.res.Matches == nil {
			continue
		}

		res[r.repo] = &repoResult{r.res, labelsOf(idx[r.repo])}
		*filesOpened += r.res.FilesOpened
	}

	*duration = int(time.Now().Sub(startedAt).Seconds() * 1000) //nolint

	return res, nil
}

// Used for parsing flags from form values.
func parseAsBool(v string) bool {
	v = strings.ToLower(v)
	return v == "true" || v == "1" || v == "fosho"
}

func parseAsRepoList(v string, idx map[string]*searcher.Searcher) []string {
	v = strings.TrimSpace(v)
	var repos []string
	if v == "*" {
		for repo := range idx {
			repos = append(repos, repo)
		}
		return repos
	}

	for _, repo := range strings.Split(v, ",") {
		if idx[repo] == nil {
			continue
		}
		repos = append(repos, repo)
	}
	return repos
}

func parseLabels(v string) map[string]bool {
	want := map[string]bool{}
	for _, label := range strings.Split(v, ",") {
		if label = strings.TrimSpace(label); label != "" {
			want[label] = true
		}
	}
	return want
}

func sortedKeys(set map[string]bool) []string {
	var keys []string
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Narrows repos to those carrying at least one label in want. An empty want
// leaves repos unchanged.
func filterByLabels(repos []string, want map[string]bool, idx map[string]*searcher.Searcher) []string {
	if len(want) == 0 {
		return repos
	}

	var filtered []string
	for _, repo := range repos {
		for _, label := range idx[repo].Repo.Labels {
			if want[label] {
				filtered = append(filtered, repo)
				break
			}
		}
	}
	return filtered
}

func labelsInUse(idx map[string]*searcher.Searcher) map[string]bool {
	inUse := map[string]bool{}
	for _, srch := range idx {
		for _, label := range srch.Repo.Labels {
			inUse[label] = true
		}
	}
	return inUse
}

// Resolves the repos and labels query parameters to the repos to search.
// Without repos, labels select from every repo. A label no repo carries, or
// labels that match none of the named repos, is an error rather than an empty
// result, which would look exactly like a search that found nothing.
func selectRepos(reposValue, labelsValue string, idx map[string]*searcher.Searcher, inUse map[string]bool) ([]string, error) {
	want := parseLabels(labelsValue)
	if strings.TrimSpace(reposValue) == "" && len(want) > 0 {
		reposValue = "*"
	}

	var unknown []string
	for _, label := range sortedKeys(want) {
		if !inUse[label] {
			unknown = append(unknown, label)
		}
	}
	if len(unknown) > 0 {
		hint := "no repo has labels"
		if len(inUse) > 0 {
			hint = "labels in use: " + strings.Join(sortedKeys(inUse), ", ")
		}
		return nil, fmt.Errorf("unknown label(s): %s (%s)", strings.Join(unknown, ", "), hint)
	}

	candidates := parseAsRepoList(reposValue, idx)
	repos := filterByLabels(candidates, want, idx)
	if len(candidates) > 0 && len(repos) == 0 {
		return nil, fmt.Errorf("none of the selected repos has label(s): %s", strings.Join(sortedKeys(want), ", "))
	}
	return repos, nil
}

func parseAsUintValue(sv string, min, max, def uint) uint {
	iv, err := strconv.ParseUint(sv, 10, 54)
	if err != nil {
		return def
	}
	if max != 0 && uint(iv) > max {
		return max
	}
	if min != 0 && uint(iv) < min {
		return min
	}
	return uint(iv)
}

func parseAsIntValue(sv string, min, max, def int) int {
	iv, err := strconv.ParseInt(sv, 10, 64)
	if err != nil {
		return def
	}
	if max != 0 && int(iv) > max {
		return max
	}
	if min != 0 && int(iv) < min {
		return min
	}
	return int(iv)
}

func parseRangeInt(v string, i *int) {
	*i = 0
	if v == "" {
		return
	}

	vi, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return
	}

	*i = int(vi)
}

func parseRangeValue(rv string) (int, int) {
	ix := strings.Index(rv, ":")
	if ix < 0 {
		return 0, 0
	}

	var b, e int
	parseRangeInt(rv[:ix], &b)
	parseRangeInt(rv[ix+1:], &e)
	return b, e
}

func Setup(m *http.ServeMux, idx map[string]*searcher.Searcher, defaultMaxResults int) {
	inUse := labelsInUse(idx)

	m.HandleFunc("/api/v1/repos", func(w http.ResponseWriter, r *http.Request) {
		res := map[string]*config.Repo{}
		for name, srch := range idx {
			res[name] = srch.Repo
		}

		writeResp(w, res)
	})

	m.HandleFunc("/api/v1/search", func(w http.ResponseWriter, r *http.Request) {
		var opt index.SearchOptions

		stats := parseAsBool(r.FormValue("stats"))
		repos, err := selectRepos(r.FormValue("repos"), r.FormValue("labels"), idx, inUse)
		if err != nil {
			writeError(w, err, http.StatusOK)
			return
		}
		query := r.FormValue("q")
		opt.Offset, opt.Limit = parseRangeValue(r.FormValue("rng"))
		opt.FileRegexp = r.FormValue("files")
		opt.ExcludeFileRegexp = r.FormValue("excludeFiles")
		opt.IgnoreCase = parseAsBool(r.FormValue("i"))
		opt.LiteralSearch = parseAsBool(r.FormValue("literal"))
		opt.MaxResults = parseAsIntValue(
			r.FormValue("limit"),
			-1,
			maxLimit,
			defaultMaxResults)
		opt.LinesOfContext = parseAsUintValue(
			r.FormValue("ctx"),
			0,
			maxLinesOfContext,
			defaultLinesOfContext)

		var filesOpened int
		var durationMs int

		results, err := searchAll(query, &opt, repos, idx, &filesOpened, &durationMs)
		if err != nil {
			// TODO(knorton): Return ok status because the UI expects it for now.
			writeError(w, err, http.StatusOK)
			return
		}

		var res struct {
			Results map[string]*repoResult
			Stats   *Stats `json:",omitempty"`
		}

		res.Results = results
		if stats {
			res.Stats = &Stats{
				FilesOpened: filesOpened,
				Duration:    durationMs,
			}
		}

		writeResp(w, &res)
	})

	m.HandleFunc("/api/v1/excludes", func(w http.ResponseWriter, r *http.Request) {
		repo := r.FormValue("repo")
		res := idx[repo].GetExcludedFiles()
		w.Header().Set("Content-Type", "application/json;charset=utf-8")
		w.Header().Set("Access-Control-Allow", "*")
		_, _ = fmt.Fprint(w, res)
	})

	m.HandleFunc("/api/v1/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			writeError(w,
				errors.New(http.StatusText(http.StatusMethodNotAllowed)),
				http.StatusMethodNotAllowed)
			return
		}

		repos := parseAsRepoList(r.FormValue("repos"), idx)

		for _, repo := range repos {
			searcher := idx[repo]
			if searcher == nil {
				writeError(w,
					fmt.Errorf("no such repository: %s", repo),
					http.StatusNotFound)
				return
			}

			if !searcher.Update() {
				writeError(w,
					fmt.Errorf("push updates are not enabled for repository %s", repo),
					http.StatusForbidden)
				return

			}
		}

		writeResp(w, "ok")
	})

	m.HandleFunc("/api/v1/github-webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			writeError(w,
				errors.New(http.StatusText(http.StatusMethodNotAllowed)),
				http.StatusMethodNotAllowed)
			return
		}

		type Webhook struct {
			Repository struct {
				Name      string
				Full_name string
			}
		}

		var h Webhook

		err := json.NewDecoder(r.Body).Decode(&h)
		if err != nil {
			writeError(w,
				errors.New(http.StatusText(http.StatusBadRequest)),
				http.StatusBadRequest)
			return
		}

		repo := h.Repository.Full_name

		searcher := idx[h.Repository.Full_name]

		if searcher == nil {
			writeError(w,
				fmt.Errorf("no such repository: %s", repo),
				http.StatusNotFound)
			return
		}

		if !searcher.Update() {
			writeError(w,
				fmt.Errorf("push updates are not enabled for repository %s", repo),
				http.StatusForbidden)
			return
		}

		writeResp(w, "ok")
	})
}
