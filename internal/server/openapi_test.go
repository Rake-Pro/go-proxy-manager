package server

import (
	"os"
	"regexp"
	"sort"
	"testing"

	openapidoc "github.com/Rake-Pro/go-proxy-manager/docs/api"
	"gopkg.in/yaml.v3"
)

// TestOpenAPISpecParses checks the embedded spec is well-formed YAML with the
// shape every OpenAPI 3.x document must have.
func TestOpenAPISpecParses(t *testing.T) {
	if len(openapidoc.Spec) == 0 {
		t.Fatal("embedded openapi spec is empty")
	}
	var doc map[string]any
	if err := yaml.Unmarshal(openapidoc.Spec, &doc); err != nil {
		t.Fatalf("openapi.yaml does not parse as YAML: %v", err)
	}
	v, ok := doc["openapi"].(string)
	if !ok || v == "" || v[0] != '3' {
		t.Fatalf("openapi.yaml: expected an \"openapi: 3.x.x\" version field, got %v", doc["openapi"])
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatal("openapi.yaml: expected a non-empty top-level \"paths\" map")
	}
}

// route is one HTTP method + path pair the running admin server actually
// registers.
type route struct {
	method string
	path   string
}

// TestOpenAPISpecCoversRegisteredRoutes derives, from the SOURCE of
// internal/server/server.go and internal/api/api.go, every route the daemon
// registers, and checks each one appears in docs/api/openapi.yaml with the
// matching HTTP method. This is deliberately a source scrape rather than a
// live mux walk: http.ServeMux exposes no route-enumeration API, and scraping
// the exact two registration sites this repo's CLAUDE.md requires be kept in
// sync with the docs is a more direct check that the docs actually describe
// what was just edited.
func TestOpenAPISpecCoversRegisteredRoutes(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(openapidoc.Spec, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}

	routes := registeredRoutes(t)
	if len(routes) == 0 {
		t.Fatal("scraped zero routes from source - the regexes below are almost certainly stale")
	}

	var missing []string
	for _, rt := range routes {
		methods, ok := doc.Paths[rt.path]
		if !ok {
			missing = append(missing, rt.method+" "+rt.path+" (path absent from openapi.yaml)")
			continue
		}
		if _, ok := methods[toLowerMethod(rt.method)]; !ok {
			missing = append(missing, rt.method+" "+rt.path+" (path present, method "+rt.method+" missing)")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("docs/api/openapi.yaml is missing %d of %d registered routes:\n%s",
			len(missing), len(routes), joinLines(missing))
	}
}

func toLowerMethod(m string) string {
	switch m {
	case "GET":
		return "get"
	case "POST":
		return "post"
	case "PUT":
		return "put"
	case "DELETE":
		return "delete"
	case "HEAD":
		return "head"
	}
	return m
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += "  - " + l + "\n"
	}
	return out
}

// handleFuncRe matches mux.HandleFunc("METHOD /path", ...) / mux.Handle("METHOD
// /path", ...) literals in server.go and the non-resource routes in api.go.
var handleFuncRe = regexp.MustCompile(`mux\.Handle(?:Func)?\("(GET|POST|PUT|DELETE|HEAD) ([^"]+)"`)

// bareHandleRe matches the pprof mounts in server.go, which carry no method
// prefix (net/http/pprof handlers respond to GET).
var bareHandleRe = regexp.MustCompile(`mux\.Handle\("(/debug/pprof[^"]*)"`)

// registerRe matches register(mux, d, "<plural>", resource[model.X]{...}) calls
// in api.go, which is the source of the six-route CRUD template below.
var registerRe = regexp.MustCompile(`register\(mux, d, "([a-z-]+)"`)

// registeredRoutes reads internal/server/server.go and internal/api/api.go
// directly (paths relative to this package's directory, which is how `go
// test` sets the working directory) and returns every route they register, in
// server-external form (i.e. with the "/api" prefix api.go's own mux does not
// carry, since internal/server mounts it with http.StripPrefix("/api", ...)).
func registeredRoutes(t *testing.T) []route {
	t.Helper()

	var routes []route

	serverSrc := readSourceFile(t, "server.go")
	for _, m := range handleFuncRe.FindAllStringSubmatch(serverSrc, -1) {
		routes = append(routes, route{method: m[1], path: m[2]})
	}
	for _, m := range bareHandleRe.FindAllStringSubmatch(serverSrc, -1) {
		routes = append(routes, route{method: "GET", path: m[1]})
	}

	apiSrc := readSourceFile(t, "../api/api.go")
	for _, m := range handleFuncRe.FindAllStringSubmatch(apiSrc, -1) {
		routes = append(routes, route{method: m[1], path: "/api" + m[2]})
	}
	// runtime.go registers the read-only runtime probe and the webhook
	// status/test routes on the same mux, so it is scraped too - otherwise a route
	// escapes the coverage gate simply by living in a second file.
	runtimeSrc := readSourceFile(t, "../api/runtime.go")
	for _, m := range handleFuncRe.FindAllStringSubmatch(runtimeSrc, -1) {
		routes = append(routes, route{method: m[1], path: "/api" + m[2]})
	}
	for _, m := range registerRe.FindAllStringSubmatch(apiSrc, -1) {
		plural := m[1]
		base := "/api/" + plural
		item := base + "/{name}"
		routes = append(routes,
			route{"GET", base},
			route{"GET", item},
			route{"PUT", item},
			route{"DELETE", item},
			route{"GET", item + "/history"},
			route{"POST", item + "/revert"},
		)
	}

	return routes
}

func readSourceFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
