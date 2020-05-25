package mixer

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type (
	// tree represents the tree data structure that contains parts of URL.
	// insert operation: /a/b/c/ URL into nodes a, b, c, /
	// search operation: depth-first search
	tree struct {
		root *node
	}

	// node represents the set of http.Handler and can be "typed".
	// Different nodes obey the next rules:
	// 	   `*` | `:` | `/`  , where `:` - path param, `/` - trailing slash, `*` - other
	// 	0)  0  |  0  |  0  -> node ready to be set
	// 	1)  0  |  1  |  0  -> only one `:` per node
	// 	2)  0  |  1  |  1  -> combination `:` and `/` allowed
	// 	3)  1  |  0  |  0  -> any combination of `*` per node
	// 	4)  1  |  0  |  1  -> combination `*` and `/` allowed
	node struct {
		tid      int
		conv     *convert
		Methods  map[string]http.Handler `json:"methods"`
		Children map[string]*node        `json:"children"`
	}

	convert func(string) (interface{}, error)
)

// pathToken determines delimiter for splitting URL parts.
const pathToken = "/"

// typeToken determines special token for URL path params.
const typeToken = ":"

const (
	other = iota // other `*`
	param        // path param `:`
	slash        // trailing slash `/`
	root         // only for tree.root node
)

// intConv adapts interface of the type conversion function from string to int.
func intConv(s string) (interface{}, error) {
	return strconv.Atoi(s)
}

// strConv adapts interface of the type conversion function from string to string.
func strConv(s string) (interface{}, error) {
	return s, nil
}

// splitURL splits incoming url to parts separated by pathToken.
// Any trailing slash will be a part too. The root path is ignored.
// If error occurred parts will return anyway.
func splitURL(url string) ([]string, error) {
	if !strings.HasPrefix(url, pathToken) {
		return []string{}, ErrPattern
	}

	parts := strings.Split(url[1:], pathToken)

	if parts[len(parts)-1] == "" {
		parts[len(parts)-1] = pathToken
	}

	for _, part := range parts {
		if part == "" {
			return parts, ErrPattern
		}
	}

	return parts, nil
}

// deepcopy returns full copy of the receiver tree.
// For conv and Methods stores only links because if
// insert operation was correct copy can replace origin.
func (t *tree) deepcopy() *tree {
	// t.root == nil is an unexpected
	return &tree{root: t.root.deepcopy()}
}

// deepcopy returns full copy from the receiver node.
// For conv and Methods stores only links because if
// insert operation was correct copy can replace origin.
func (n *node) deepcopy() *node {
	// n == nil is an unexpected
	c := new(node)
	*c = *n

	if n.Children != nil {
		c.Children = make(map[string]*node)
	}

	for k, v := range n.Children {
		c.Children[k] = v.deepcopy()
	}

	return c
}

// find finds child node by type ID.
func (n *node) find(tid int) *node {
	for _, c := range n.Children {
		if c.tid == tid {
			return c
		}
	}

	return nil
}

// insert inserts new node starting from n and returns true.
// Returns false if one of the node rules broken.
func (n *node) insert(key string, in *node) bool {
	var found *node

	switch in.tid {
	case param:
		found = n.find(other)
	case other:
		found = n.find(param)
	}

	if found != nil {
		return false
	}

	if n.Children == nil {
		n.Children = make(map[string]*node)
	}

	n.Children[key] = in
	return true
}

// insert builds parts to inner tree by some rules:
// 	 - if node for part not exist it will be created.
// 	 - if node for part exist it will be returned.
// Returns methods associated with last inserted or found node.
func (mix *ServeMixer) insert(parts []string) (map[string]http.Handler, error) {
	copy_ := mix.tree.deepcopy()
	curr := copy_.root

	for _, part := range parts {
		in := new(node)

		switch part[:1] {
		case pathToken:
			in.tid = slash
		case typeToken:
			conv := mix.converters[part[1:]]

			if conv == nil {
				return nil, ErrPathParam
			}

			in.tid = param
			in.conv = conv

			part = typeToken
		}

		child, ok := curr.Children[part]
		if ok && child.conv != in.conv {
			return nil, ErrMultiplePathParam
		}

		if ok {
			curr = child
			continue
		}

		if !curr.insert(part, in) {
			return nil, ErrMultiplePathParam
		}

		curr = in
	}

	if curr.Methods == nil {
		curr.Methods = make(map[string]http.Handler)
	}

	*mix.tree = *copy_

	return curr.Methods, nil
}

// Handle adds handler by pattern and Handle.
// Because it is an initialization moment will be panics in any error.
func (mix *ServeMixer) Handle(method, pattern string, handler http.Handler) {
	switch method {
	case
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodOptions:
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

	methods, err := mix.insert(parts)
	if err != nil {
		panic(&ServeMixerError{method, pattern, err})
	}

	if methods[method] != nil {
		panic(duplicateError(method, pattern))
	}

	methods[method] = handler
}

func (mix *ServeMixer) HandleFunc(method, pattern string, handler func(http.ResponseWriter, *http.Request)) {
	if handler == nil {
		panic(handlerError(method, pattern))
	}
	mix.Handle(method, pattern, http.HandlerFunc(handler))
}

func (mix *ServeMixer) Handler(r *http.Request) (http.Handler, error) {
	url := r.URL.EscapedPath()
	parts, _ := splitURL(url)

	i := 0
	node := mix.tree.root
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

// PathParams represents map of path params that will store by index.
type PathParams map[int]interface{}

type contextKey struct {
	name string
}

// PathParamsCtxKey is a context key for using in context.Value.
var PathParamsCtxKey = &contextKey{"path-params"}

func GetPathParams(r *http.Request) PathParams {
	params, ok := r.Context().Value(PathParamsCtxKey).(PathParams)

	if !ok {
		return nil
	}

	return params
}

// ServeHTTP implements a Handler's interface.
func (mix *ServeMixer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h, err := mix.Handler(r)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}

	h.ServeHTTP(w, r)
}

// ServeMixerError decorates all possible external errors to one kind.
type ServeMixerError struct {
	method  string
	pattern string
	err     error
}

// Error implements the error's Error.
func (e *ServeMixerError) Error() string {
	return "mixer: handler (" + e.method + ") " + e.pattern + " error: " + e.err.Error()
}

// Unwrap implements the error's Unwrap.
func (e *ServeMixerError) Unwrap() error {
	return e.err
}

// ErrMethod is the error if try set handler for wrong method inside ServeMixer.
var ErrMethod = errors.New("invalid method")

func methodError(m, p string) *ServeMixerError {
	return &ServeMixerError{m, p, ErrMethod}
}

// ErrHandler is the error if try set nil handler inside ServeMixer.
var ErrHandler = errors.New("nil handler")

func handlerError(m, p string) *ServeMixerError {
	return &ServeMixerError{m, p, ErrHandler}
}

// ErrPattern is the error if get invalid URL pattern.
var ErrPattern = errors.New("invalid pattern")

func patternError(m, p string) *ServeMixerError {
	return &ServeMixerError{m, p, ErrPattern}
}

// ErrDuplicate is the error if get multiple handlers for one method.
var ErrDuplicate = errors.New("duplicate handler")

func duplicateError(m, p string) *ServeMixerError {
	return &ServeMixerError{m, p, ErrDuplicate}
}

// ErrNotFound is the error if handler for combination method + pattern not exist.
var ErrNotFound = errors.New("not found")

func notFoundError(m, p string) *ServeMixerError {
	return &ServeMixerError{m, p, ErrNotFound}
}

// ErrPathParam signals that typed path param is invalid.
var ErrPathParam = errors.New("invalid path param")

// ErrMultiplePathParam signals that insert tried perform
// operation on invalid rule: for more see definition of node.
var ErrMultiplePathParam = errors.New("multiple types for path param")

// ServeMixer is an HTTP request multiplexer.
// FIXME: ADD DESCRIPTION LIKE http.ServeMux
type ServeMixer struct {
	tree       *tree
	converters map[string]*convert
}

// NewServeMixer allocates and returns a new ServeMixer.
func NewServeMixer() *ServeMixer {
	sc := convert(strConv)
	ic := convert(intConv)

	return &ServeMixer{
		tree: &tree{root: &node{tid: root}},
		converters: map[string]*convert{
			"":    &sc,
			"str": &sc,
			"int": &ic,
		},
	}
}

func (mix *ServeMixer) Get(pattern string, handler http.Handler) {
	mix.Handle(http.MethodGet, pattern, handler)
}

func (mix *ServeMixer) GetFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mix.HandleFunc(http.MethodGet, pattern, handler)
}

func (mix *ServeMixer) Head(pattern string, handler http.Handler) {
	mix.Handle(http.MethodHead, pattern, handler)
}

func (mix *ServeMixer) HeadFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mix.HandleFunc(http.MethodHead, pattern, handler)
}

func (mix *ServeMixer) Post(pattern string, handler http.Handler) {
	mix.Handle(http.MethodPost, pattern, handler)
}

func (mix *ServeMixer) PostFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mix.HandleFunc(http.MethodPost, pattern, handler)
}

func (mix *ServeMixer) Put(pattern string, handler http.Handler) {
	mix.Handle(http.MethodPut, pattern, handler)
}

func (mix *ServeMixer) PutFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mix.HandleFunc(http.MethodPut, pattern, handler)
}

func (mix *ServeMixer) Patch(pattern string, handler http.Handler) {
	mix.Handle(http.MethodPatch, pattern, handler)
}

func (mix *ServeMixer) PatchFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mix.HandleFunc(http.MethodPatch, pattern, handler)
}

func (mix *ServeMixer) Delete(pattern string, handler http.Handler) {
	mix.Handle(http.MethodDelete, pattern, handler)
}

func (mix *ServeMixer) DeleteFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mix.HandleFunc(http.MethodDelete, pattern, handler)
}

func (mix *ServeMixer) Options(pattern string, handler http.Handler) {
	mix.Handle(http.MethodOptions, pattern, handler)
}

func (mix *ServeMixer) OptionsFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	mix.HandleFunc(http.MethodOptions, pattern, handler)
}
