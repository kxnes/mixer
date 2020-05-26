// Package mixer contains the HTTP multiplexer based on similar radix tree data structure.
package mixer

import (
	"context"
	"errors"
	"net/http"
)

type (
	// PathParams represents map of path params that will store by index.
	PathParams map[int]interface{}

	// ServeMuxError decorates all possible external errors to one kind.
	ServeMuxError struct {
		method  string
		pattern string
		err     error
	}

	// ServeMux is an HTTP request multiplexer.
	ServeMux struct {
		tree       *tree
		converters map[string]*convert
	}
)

var (
	// PathParamsCtxKey is a context key for using in context.Value.
	PathParamsCtxKey = &contextKey{"path-params"}

	// ErrMethod is the error if try set handler for wrong method inside ServeMux.
	ErrMethod = errors.New("invalid method")

	// ErrHandler is the error if try set nil handler inside ServeMux.
	ErrHandler = errors.New("nil handler")

	// ErrPattern is the error if get invalid URL pattern.
	ErrPattern = errors.New("invalid pattern")

	// ErrDuplicate is the error if get multiple handlers for one method.
	ErrDuplicate = errors.New("duplicate handler")

	// ErrNotFound is the error if handler for combination method + pattern not exist.
	ErrNotFound = errors.New("not found")

	// ErrPathParam signals that typed path param is invalid.
	ErrPathParam = errors.New("invalid path param")

	// ErrMultiplePathParam signals that insert tried perform
	// operation on invalid rule: for more see definition of node.
	ErrMultiplePathParam = errors.New("multiple types for path param")
)

// Error implements the error's Error.
func (e *ServeMuxError) Error() string {
	return "httpmux: handler (" + e.method + ") " + e.pattern + " error: " + e.err.Error()
}

// Unwrap implements the error's Unwrap.
func (e *ServeMuxError) Unwrap() error {
	return e.err
}

// GetPathParams returns the path params registered in r.Context() or nil otherwise.
func GetPathParams(r *http.Request) PathParams {
	params, ok := r.Context().Value(PathParamsCtxKey).(PathParams)

	if !ok {
		return nil
	}

	return params
}

// Handler returns the handler to use for the given request.
func (mux *ServeMux) Handler(r *http.Request) (http.Handler, error) {
	url := r.URL.EscapedPath()
	parts, _ := splitURL(url)

	i := 0
	node := mux.tree.root
	params := make(PathParams)

	for _, part := range parts {
		child, ok := node.Children[part]
		if ok {
			node = child
			continue
		}

		child, ok = node.Children[typeToken]
		if !ok {
			return nil, notFoundError(r.Method, url)
		}

		val, err := (*child.conv)(part)
		if err != nil {
			return nil, notFoundError(r.Method, url)
		}

		node = child
		params[i] = val
		i++
	}

	if node.Methods == nil || node.Methods[r.Method] == nil {
		return nil, notFoundError(r.Method, url)
	}

	if len(params) != 0 {
		*r = *r.WithContext(context.WithValue(r.Context(), PathParamsCtxKey, params))
	}

	return node.Methods[r.Method], nil
}

// Handle registers the handler for the given method and pattern.
// Because it is an initialization moment will be panics in any error.
func (mux *ServeMux) Handle(method, pattern string, handler http.Handler) {
	switch method {
	case
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodConnect,
		http.MethodOptions,
		http.MethodTrace:
	default:
		panic(methodError(method, pattern))
	}

	if handler == nil {
		panic(handlerError(method, pattern))
	}

	parts, err := splitURL(pattern)
	if err != nil {
		panic(patternError(method, pattern))
	}

	methods, err := mux.insert(parts)
	if err != nil {
		panic(&ServeMuxError{method, pattern, err})
	}

	if methods[method] != nil {
		panic(duplicateError(method, pattern))
	}

	methods[method] = handler
}

// Get registers the GET handler for the given pattern.
func (mux *ServeMux) Get(pattern string, handler http.Handler) {
	mux.Handle(http.MethodGet, pattern, handler)
}

// Head registers the HEAD handler for the given pattern.
func (mux *ServeMux) Head(pattern string, handler http.Handler) {
	mux.Handle(http.MethodHead, pattern, handler)
}

// Post registers the POST handler for the given pattern.
func (mux *ServeMux) Post(pattern string, handler http.Handler) {
	mux.Handle(http.MethodPost, pattern, handler)
}

// Put registers the PUT handler for the given pattern.
func (mux *ServeMux) Put(pattern string, handler http.Handler) {
	mux.Handle(http.MethodPut, pattern, handler)
}

// Patch registers the PATCH handler for the given pattern.
func (mux *ServeMux) Patch(pattern string, handler http.Handler) {
	mux.Handle(http.MethodPatch, pattern, handler)
}

// Delete registers the DELETE handler for the given pattern.
func (mux *ServeMux) Delete(pattern string, handler http.Handler) {
	mux.Handle(http.MethodDelete, pattern, handler)
}

// Connect registers the CONNECT handler for the given pattern.
func (mux *ServeMux) Connect(pattern string, handler http.Handler) {
	mux.Handle(http.MethodConnect, pattern, handler)
}

// Options registers the OPTIONS handler for the given pattern.
func (mux *ServeMux) Options(pattern string, handler http.Handler) {
	mux.Handle(http.MethodOptions, pattern, handler)
}

// Trace registers the TRACE handler for the given pattern.
func (mux *ServeMux) Trace(pattern string, handler http.Handler) {
	mux.Handle(http.MethodTrace, pattern, handler)
}

// HandleFunc registers the handler function for the given method and pattern.
func (mux *ServeMux) HandleFunc(method, pattern string, handler func(http.ResponseWriter, *http.Request)) {
	if handler == nil {
		panic(handlerError(method, pattern))
	}

	mux.Handle(method, pattern, http.HandlerFunc(handler))
}

// GetFunc registers the GET handler function for the given pattern.
func (mux *ServeMux) GetFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mux.HandleFunc(http.MethodGet, pattern, handler)
}

// HeadFunc registers the HEAD handler function for the given pattern.
func (mux *ServeMux) HeadFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mux.HandleFunc(http.MethodHead, pattern, handler)
}

// PostFunc registers the POST handler function for the given pattern.
func (mux *ServeMux) PostFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mux.HandleFunc(http.MethodPost, pattern, handler)
}

// PutFunc registers the PUT handler function for the given pattern.
func (mux *ServeMux) PutFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mux.HandleFunc(http.MethodPut, pattern, handler)
}

// PatchFunc registers the PATCH handler function for the given pattern.
func (mux *ServeMux) PatchFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mux.HandleFunc(http.MethodPatch, pattern, handler)
}

// DeleteFunc registers the DELETE handler function for the given pattern.
func (mux *ServeMux) DeleteFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mux.HandleFunc(http.MethodDelete, pattern, handler)
}

// ConnectFunc registers the CONNECT handler function for the given pattern.
func (mux *ServeMux) ConnectFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mux.HandleFunc(http.MethodConnect, pattern, handler)
}

// OptionsFunc registers the OPTIONS handler function for the given pattern.
func (mux *ServeMux) OptionsFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mux.HandleFunc(http.MethodOptions, pattern, handler)
}

// TraceFunc registers the TRACE handler function for the given pattern.
func (mux *ServeMux) TraceFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mux.HandleFunc(http.MethodTrace, pattern, handler)
}

// ServeHTTP implements a Handler's interface.
func (mux *ServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h, err := mux.Handler(r)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}

	h.ServeHTTP(w, r)
}

// New allocates and returns a new ServeMux.
func New() *ServeMux {
	sc := convert(strConv)
	ic := convert(intConv)

	return &ServeMux{
		tree: &tree{root: &node{tid: root}},
		converters: map[string]*convert{
			"":    &sc,
			"str": &sc,
			"int": &ic,
		},
	}
}
