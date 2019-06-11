// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	// For triggering manual rebuilds.
	"golang.org/x/oauth2/google"
	"google.golang.org/api/cloudbuild/v1"
)

var redirects = map[string]string{
	"/change":    "https://github.com/google/gvisor",
	"/issue":     "https://github.com/google/gvisor/issues",
	"/issue/new": "https://github.com/google/gvisor/issues/new",
	"/pr":        "https://github.com/google/gvisor/pulls",

	// Redirects to compatibility docs.
	"/c":             "/docs/user_guide/compatibility",
	"/c/linux/amd64": "/docs/user_guide/compatibility/amd64",

	// Deprecated, but links continue to work.
	"/cl": "https://gvisor-review.googlesource.com",
}

var prefixHelpers = map[string]string{
	"change": "https://github.com/google/gvisor/commit/%s",
	"issue":  "https://github.com/google/gvisor/issues/%s",
	"pull":   "https://github.com/google/gvisor/pull/%s",

	// Redirects to compatibility docs.
	"c/linux/amd64": "/docs/user_guide/compatibility/amd64/#%s",

	// Redirect to the source viewer.
	"gvisor": "https://github.com/google/gvisor/tree/go/%s",

	// Deprecated, but links continue to work.
	"cl": "https://gvisor-review.googlesource.com/c/gvisor/+/%s",
}

var (
	validId     = regexp.MustCompile(`^[A-Za-z0-9-]*/?$`)
	goGetHeader = `<meta name="go-import" content="gvisor.dev/gvisor git https://gvisor.dev/gvisor">`
	goGetHTML5  = `<!doctype html><html><head><meta charset=utf-8>` + goGetHeader + `<title>Go-get</title></head><body></html>`
)

// wrappedHandler wraps an http.Handler.
//
// If the query parameters include go-get=1, then we redirect to a single
// static page that allows us to serve arbitrary Go packages.
func wrappedHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gg, ok := r.URL.Query()["go-get"]
		if ok && len(gg) == 1 && gg[0] == "1" {
			// Serve a trivial html page.
			w.Write([]byte(goGetHTML5))
			return
		}
		// Fallthrough.
		h.ServeHTTP(w, r)
	})
}

// redirectWithQuery redirects to the given target url preserving query parameters.
func redirectWithQuery(w http.ResponseWriter, r *http.Request, target string) {
	url := target
	if qs := r.URL.RawQuery; qs != "" {
		url += "?" + qs
	}
	http.Redirect(w, r, url, http.StatusFound)
}

// hostRedirectHandler redirects the www. domain to the naked domain.
func hostRedirectHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Host, "www.") {
			// Redirect to the naked domain.
			r.URL.Scheme = "https"  // Assume https.
			r.URL.Host = r.Host[4:] // Remove the 'www.'
			http.Redirect(w, r, r.URL.String(), http.StatusMovedPermanently)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// prefixRedirectHandler returns a handler that redirects to the given formated url.
func prefixRedirectHandler(prefix, baseURL string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Path; p == prefix {
			// Redirect /prefix/ to /prefix.
			http.Redirect(w, r, p[:len(p)-1], http.StatusFound)
			return
		}
		id := r.URL.Path[len(prefix):]
		if !validId.MatchString(id) {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		target := fmt.Sprintf(baseURL, id)
		redirectWithQuery(w, r, target)
	})
}

// redirectHandler returns a handler that redirects to the given url.
func redirectHandler(target string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectWithQuery(w, r, target)
	})
}

// redirectRedirects registers redirect http handlers.
func registerRedirects(mux *http.ServeMux) {
	if mux == nil {
		mux = http.DefaultServeMux
	}

	for prefix, baseURL := range prefixHelpers {
		p := "/" + prefix + "/"
		mux.Handle(p, hostRedirectHandler(wrappedHandler(prefixRedirectHandler(p, baseURL))))
	}

	for path, redirect := range redirects {
		mux.Handle(path, hostRedirectHandler(wrappedHandler(redirectHandler(redirect))))
	}
}

// registerStatic registers static file handlers
func registerStatic(mux *http.ServeMux, staticDir string) {
	if mux == nil {
		mux = http.DefaultServeMux
	}
	mux.Handle("/", hostRedirectHandler(wrappedHandler(http.FileServer(http.Dir(staticDir)))))
}

// registerRebuild registers the rebuild handler.
func registerRebuild(mux *http.ServeMux) {
	if mux == nil {
		mux = http.DefaultServeMux
	}

	mux.Handle("/rebuild", wrappedHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		credentials, err := google.FindDefaultCredentials(ctx, cloudbuild.CloudPlatformScope)
		if err != nil {
			http.Error(w, "credentials error: "+err.Error(), 500)
			return
		}
		cloudbuildService, err := cloudbuild.NewService(ctx)
		if err != nil {
			http.Error(w, "cloudbuild service error: "+err.Error(), 500)
			return
		}
		projectID := credentials.ProjectID
		if projectID == "" {
			// If running locally, then this project will not be
			// available. Use the default project here.
			projectID = "gvisor-website"
		}
		triggers, err := cloudbuildService.Projects.Triggers.List(projectID).Do()
		if err != nil {
			http.Error(w, "trigger list error: "+err.Error(), 500)
			return
		}
		if len(triggers.Triggers) < 1 {
			http.Error(w, "trigger list error: no triggers", 500)
			return
		}
		if _, err := cloudbuildService.Projects.Triggers.Run(
			projectID,
			triggers.Triggers[0].Id,
			&cloudbuild.RepoSource{
				// In the current project, require that a
				// github cloud source repository exists with
				// the given name, and build from master.
				BranchName: "master",
				RepoName:   "github_google_gvisor-website",
				ProjectId:  projectID,
			}).Do(); err != nil {
			http.Error(w, "run error: "+err.Error(), 500)
			return
		}
	})))
}

// registerRepo registers the repository handler.
func registerRepo(mux *http.ServeMux) {
	if mux == nil {
		mux = http.DefaultServeMux
	}

	mux.Handle("/gvisor/git-upload-pack", hostRedirectHandler(http.HandlerFunc(gitUploadPack)))
	mux.Handle("/gvisor/info/refs", hostRedirectHandler(http.HandlerFunc(infoRefs)))
}

const (
	upstreamInfoRefs      = "https://github.com/google/gvisor.git/info/refs?service=git-upload-pack"
	upstreamGitUploadPack = "https://github.com/google/gvisor.git/git-upload-pack"
)

// targetURL is the URL object for upstreamGitUploadPack.
var targetURL = func() *url.URL {
	url, err := url.Parse(upstreamGitUploadPack)
	if err != nil {
		panic(err) // Should never happen.
	}
	return url
}()

// targetProxy is the reverse proxy for the main repository.
var targetProxy = &httputil.ReverseProxy{
	Director: func(req *http.Request) {
		req.URL.Scheme = targetURL.Scheme
		req.URL.Host = targetURL.Host
		req.URL.Path = targetURL.Path
		req.Header.Set("Host", targetURL.Host)
	},
}

func gitUploadPack(w http.ResponseWriter, r *http.Request) {
	// Proxy to the upstream repository.
	targetProxy.ServeHTTP(w, r)
}

func infoRefs(w http.ResponseWriter, r *http.Request) {
	// We intercept the client request. We implement only the appropriate
	// git-upload-pack service and enforce that clients are asking for this
	// as well. The full specification of this service, as well as a formal
	// grammar is available at:
	//
	//	https://github.com/git/git/blob/master/Documentation/technical/http-protocol.txt
	//
	// We read only the first reference and change the primary branch. All
	// other references and branches are left alone.

	// Was this a dumb client? These are not supported.
	vs, ok := r.URL.Query()["service"]
	if !ok || len(r.URL.Query()) > 1 || len(vs) != 1 || vs[0] != "git-upload-pack" {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "invalid query: %v", r.URL.Query())
		return
	}

	// Connect upstream.
	client := new(http.Client)
	resp, err := client.Get(upstreamInfoRefs)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "upstream get error: %v", err)
		return
	}
	defer resp.Body.Close()

	// Read the full upstream contents.
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "upstream read error: %v", err)
		return
	}

	// Was there an error? Pass all data through.
	if resp.StatusCode != 200 {
		w.WriteHeader(resp.StatusCode)
		w.Write(data)
		return
	}

	// emitPkt emits a single packet line.
	emitPkt := func(m string) {
		if len(m) == 0 {
			// Special case: normally the size includes the size of
			// the four byte header. However, we see that size 0000
			// appears after the header and is the terminal. We use
			// the empty string to indicate this.
			fmt.Fprintf(w, "%04x", 0)
		} else {
			fmt.Fprintf(w, "%04x%s\n", 4+len(m)+1, m)
		}
	}

	// readPkt reads a single packet line.
	readPkt := func() (string, bool) {
		// Parse the size header and return the string.
		if len(data) < 4 {
			return "", false
		}
		size, err := strconv.ParseInt(string(data[:4]), 16, 32)
		if err != nil {
			return "", false
		}
		if size == 0 {
			data = data[4:]
			return "", true
		} else if len(data) >= int(size) {
			m := string(data[4:size])
			data = data[size:]
			return strings.TrimSuffix(m, "\n"), true
		} else {
			return "", false
		}
	}

	const (
		// serviceLine is the first line emitted.
		serviceLine = "# service=git-upload-pack"

		// target is the target branch.
		target = "refs/heads/go"
	)

	// Check the header.
	header, ok := readPkt()
	if !ok || header != serviceLine {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "invalid upstream header: %v", header)
		return
	}

	// readRef reads a single reference.
	readRef := func() (hash string, ref string, options []string, ok bool) {
		line, ok := readPkt()
		if line == "" && ok {
			return "", "", nil, true
		}
		if !ok || line == "" {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "invalid reference: %v", line)
			return "", "", nil, false
		}
		// Note that parts is guarnateed to be at least one element.
		// Per the strings.Split documentation: "If s does not contain
		// sep and sep is not empty, Split returns a slice of length 1
		// whose only element is s."
		parts := strings.Split(line, "\x00")
		if len(parts) > 2 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "invalid reference: %v", line)
			return "", "", nil, false
		}
		if len(parts) == 2 {
			options = strings.Split(parts[1], " ")
		}
		parts = strings.Split(parts[0], " ")
		if len(parts) != 2 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "invalid reference: %v", line)
			return "", "", nil, false
		}
		return parts[0], parts[1], options, true
	}

	// Read any empty blocks. This does not appear to be generally part of
	// the spec, but clients rely on at least a single blank message
	// following the header. We just implement this faithfully.
	headHash, first, options, ok := readRef()
	for first == "" && ok {
		headHash, first, options, ok = readRef()
	}
	if !ok {
		return // Already sent error.
	}

	// Rewrite the options.
	for i, option := range options {
		if strings.HasPrefix(option, "symref=") {
			// Rewrite to point to our branch target.
			options[i] = fmt.Sprintf("symref=%s", target)
		}
	}

	// Read all other references.
	others := make(map[string]string)
	order := make([]string, 0, 1)
	for {
		refHash, other, _, ok := readRef()
		if !ok {
			return // Already sent error.
		}
		if refHash == "" {
			break // Terminal.
		}
		others[other] = refHash      // Save reference.
		order = append(order, other) // Save order.
	}

	// Ensure our reference exists.
	if _, ok := others[target]; !ok {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "invalid target reference: %v", target)
		return
	}

	// If the original top-line reference was HEAD, then we need to rewrite
	// this and the possible symref link. If it was not HEAD, then we need
	// to actually change the ordering.
	if first == "HEAD" {
		headHash = others[target] // Just change the hash.
	} else {
		headHash = others[target]    // Rewrite the hash.
		first = target               // And the reference.
		others[first] = headHash     // Insert the regular reference.
		order = append(order, first) // Insert the reference to the ordering.
		sort.Strings(order)          // Sort resulting references.
		delete(others, target)       // Drop the original reference.
	}

	// Required headers per the spec.
	w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
	w.Header().Set("Cache-Control", "no-cache")
	emitPkt(serviceLine)
	emitPkt("") // See above.
	emitPkt(fmt.Sprintf("%s %s\x00%s", headHash, first, strings.Join(options, " ")))
	for _, other := range order {
		hash, ok := others[other]
		if !ok {
			// This should never happen if the above is correct.
			panic(fmt.Sprintf("invalid other reference: %v", other))
		}
		emitPkt(fmt.Sprintf("%s %s", hash, other))
	}
	emitPkt("") // Terminal.
}

func envFlagString(name, def string) string {
	if val := os.Getenv(name); val != "" {
		return val
	}
	return def
}

var (
	addr      = flag.String("http", envFlagString("HTTP", ":8080"), "HTTP service address")
	staticDir = flag.String("static-dir", envFlagString("STATIC_DIR", "static"), "static files directory")
)

func main() {
	flag.Parse()

	registerRedirects(nil)
	registerRebuild(nil)
	registerRepo(nil)
	registerStatic(nil, *staticDir)

	log.Fatal(http.ListenAndServe(*addr, nil))
}
