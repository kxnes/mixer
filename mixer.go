package mixer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// const

const (
	pathToken = "/"
	typeToken = ":"
)

// errors

// ErrServeMixer uses as a format string for `fmt.Errorf`
// for decorate all external errors to one external type.
const ErrServeMixer = "mixer: handler (%s) %s error = %w"

var (
	// ErrNotMethod is the error if get unsupported method.
	ErrNotMethod = errors.New("method not support")
	// ErrNotPattern is the error if get invalid URL pattern.
	ErrNotPattern = errors.New("invalid pattern")
	// ErrDuplicate is the error if get multiple handlers for one method.
	ErrDuplicate = errors.New("duplicate handler")
	// ErrNotFound is the error if handler for combination method + pattern not exist.
	ErrNotFound = errors.New("not found")
)

// aliases

type (
	handlerMap   = map[string]http.Handler
	converterMap = map[string]*converter
)

// types

type (
	// converter is a struct that contain converter function for path params.
	converter struct {
		fn func(string) (interface{}, error)
	}
	// treeNode represents set of handlers.
	// Uses modified version of trie data structure.
	treeNode struct {
		conv     *converter
		Methods  handlerMap           `json:"methods,omitempty"`
		Children map[string]*treeNode `json:"children,omitempty"`
	}
	// contextKey is a value for use with context.WithValue.
	contextKey struct {
		name string
	}
)

// converters

// intConv adapts type conversion from string to int for using in `converter.fn`.
func intConv(s string) (interface{}, error) {
	return strconv.Atoi(s)
}

// strConv adapts type conversion from string to string for using in `converter.fn`.
func strConv(s string) (interface{}, error) {
	return s, nil
}

// inner implementation

// splitURL splits incoming `url` to parts separated by `pathToken`.
// Any trailing slash will be a part too.
func (mix *ServeMixer) splitURL(url string) []string {
	if !strings.HasPrefix(url, pathToken) {
		return nil
	}

	parts := strings.Split(url[1:], pathToken)
	tail := len(parts) - 1

	for _, part := range parts[:tail] {
		if part == "" {
			return nil
		}
	}

	if parts[tail] == "" {
		parts[tail] = pathToken
	}

	return parts
}

// deepcopy makes and returns deep copy from calling `node`.
func (node *treeNode) deepcopy() *treeNode {
	c := &treeNode{}

	if node.Children != nil {
		c.Children = make(map[string]*treeNode)
	}

	for k, v := range node.Children {
		c.Children[k] = v.deepcopy()
	}

	if node.Methods != nil {
		c.Methods = make(handlerMap)

		for a, b := range node.Methods {
			c.Methods[a] = b
		}
	}

	c.conv = node.conv

	return c
}

// method adds `handler` by `pattern` and `method`.
// Because it is an initialization moment will be panics in any error.
func (mix *ServeMixer) method(pattern, method string, handler http.Handler) {
	switch method {
	case
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete:
	default:
		panic(fmt.Errorf(ErrServeMixer, method, pattern, ErrNotMethod))
	}

	parts := mix.splitURL(pattern)
	if parts == nil {
		panic(fmt.Errorf(ErrServeMixer, method, pattern, ErrNotPattern))
	}

	hm, err := mix.add(parts)
	if err != nil {
		panic(fmt.Errorf(ErrServeMixer, method, pattern, err))
	}

	if hm[method] != nil {
		panic(fmt.Errorf(ErrServeMixer, method, pattern, ErrDuplicate))
	}

	hm[method] = handler
}

// add adds `parts` to inner tree by some rules:
//  - if node for `part` not exist it will be created.
//  - if node for `part` exist it will be returned.
// Returns `handlerMap` associated with last inserted or found node.
//
// NOTICE: All operations will be perform on copy of `mix.root`.
// Copy will replace original `mix.root` on success.
func (mix *ServeMixer) add(parts []string) (handlerMap, error) {
	c := mix.root.deepcopy()
	node := c

	for _, part := range parts {
		var conv *converter

		if part[:1] == typeToken {
			conv = mix.converters[part[1:]]
			if conv == nil {
				return nil, errors.New("invalid path param")
			}

			part = typeToken
		}

		// CRITICAL SECTION: different nodes obey the next rules:
		//    `*` | `:` | `/`  , where `:` - path param, `/` - trailing slash, `*` - other
		// 1)  0  |  1  |  0  -> only one `:` per node
		// 2)  0  |  1  |  1  -> combination `:` and `/` allowed
		// 3)  1  |  0  |  0  -> any combination of `*` per node
		// 4)  1  |  0  |  1  -> combination `*` and `/` allowed
		child := node.Children[part]

		switch {
		// check, that 1) and 2) allowed by types
		case child != nil && child.conv != conv:
			return nil, errors.New("multiple types for path param")
		// check, that 1) or 2) allowed
		case part == typeToken:
			var cnt int

			if child != nil {
				cnt++
			}

			if node.Children[pathToken] != nil {
				cnt++
			}

			// prevent cross 2) and 4)
			if len(node.Children) > cnt {
				return nil, errors.New("multiple types for path param")
			}
		// check, that 3) allowed
		case part != pathToken && node.Children[typeToken] != nil:
			return nil, errors.New("multiple types for path param")
		}

		if child != nil {
			node = child
			continue
		}

		if node.Children == nil {
			node.Children = make(map[string]*treeNode)
		}

		node.Children[part] = &treeNode{conv: conv}
		node = node.Children[part]
	}

	*mix.root = *c

	if node.Methods == nil {
		node.Methods = make(handlerMap)
	}

	return node.Methods, nil
}

func (mix *ServeMixer) handler(r *http.Request) (http.Handler, error) {
	parts := strings.Split(r.URL.EscapedPath()[1:], pathToken)
	tail := len(parts) - 1

	if parts[tail] == "" {
		parts[tail] = pathToken
	}

	i := 0
	node := mix.root
	params := make(PathParams)

	for _, part := range parts {
		child, ok := node.Children[part]
		if ok {
			node = child
			continue
		}

		child, ok = node.Children[typeToken]
		if !ok {
			if mix.strict {
				return nil, ErrNotFound
			}
			break
		}

		val, err := child.conv.fn(part)
		if err != nil {
			return nil, ErrNotFound
		}

		node = child
		params[i] = val
		i++
	}

	found := node.Methods != nil && node.Methods[r.Method] != nil

	if !mix.strict && !found && node.Children[pathToken] != nil {
		node = node.Children[pathToken]
		found = node.Methods[r.Method] != nil
	}

	if !found {
		return nil, ErrNotFound
	}

	if len(params) != 0 {
		*r = *r.WithContext(context.WithValue(r.Context(), PathParamsCtxKey, params))
	}

	return node.Methods[r.Method], nil
}

// API

// ServeMixer is an HTTP request multiplexer.
// FIXME: ADD DESCRIPTION LIKE http.ServeMux
type ServeMixer struct {
	strict bool
	root       *treeNode
	converters converterMap
}

// PathParams represents map of path params that will store by index.
type PathParams map[int]interface{}

// PathParamsCtxKey is a context key for using in context.Value.
var PathParamsCtxKey = &contextKey{"path-params"}

func GetPathParams(r *http.Request) PathParams {
	return r.Context().Value(PathParamsCtxKey).(PathParams)
}

// ServeHTTP implements a Handler's interface.
func (mix *ServeMixer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h, err := mix.handler(r)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}

	h.ServeHTTP(w, r)
}

// Get registers the GET `handler` for the given `pattern`.
func (mix *ServeMixer) Get(pattern string, handler http.Handler) {
	mix.method(pattern, http.MethodGet, handler)
}

// Post registers the POST `handler` for the given `pattern`.
func (mix *ServeMixer) Post(pattern string, handler http.Handler) {
	mix.method(pattern, http.MethodPost, handler)
}

// Put registers the PUT `handler` for the given `pattern`.
func (mix *ServeMixer) Put(pattern string, handler http.Handler) {
	mix.method(pattern, http.MethodPut, handler)
}

// Patch registers the PATCH `handler` for the given `pattern`.
func (mix *ServeMixer) Patch(pattern string, handler http.Handler) {
	mix.method(pattern, http.MethodPatch, handler)
}

// Delete registers the DELETE `handler` for the given `pattern`.
func (mix *ServeMixer) Delete(pattern string, handler http.Handler) {
	mix.method(pattern, http.MethodDelete, handler)
}

// newServeMixer allocates and returns a new ServeMixer.
func newServeMixer(strictSlashes bool) *ServeMixer {
	strConv := converter{fn: strConv}

	return &ServeMixer{
		strict: strictSlashes,
		root: new(treeNode),
		converters: map[string]*converter{
			"":    &strConv,
			"str": &strConv,
			"int": {fn: intConv},
		},
	}
}

// NewStrictServeMixer returns ServeMixer that
// has strict behaviour while  capturing request.
func NewStrictServeMixer() *ServeMixer {
	return newServeMixer(true)
}

// NewDanglingServeMixer returns ServeMixer that
// has not behaviour while capturing request.
// There can be a couple side-effects because it is dangling.
func NewDanglingServeMixer() *ServeMixer {
	return newServeMixer(false)
}
