package mixer

import (
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

	// convert represents the convert function for path params.
	convert func(string) (interface{}, error)

	// contextKey is a value for use with context.WithValue.
	contextKey struct {
		name string
	}
)

const (
	other = iota // other `*`
	param        // path param `:`
	slash        // trailing slash `/`
	root         // only for tree.root node

	// pathToken determines delimiter for splitting URL parts.
	pathToken = "/"

	// typeToken determines special token for URL path params.
	typeToken = ":"
)

// methodError wraps the ErrMethod error.
func methodError(m, p string) *ServeMuxError {
	return &ServeMuxError{m, p, ErrMethod}
}

// handlerError wraps the ErrHandler error.
func handlerError(m, p string) *ServeMuxError {
	return &ServeMuxError{m, p, ErrHandler}
}

// patternError wraps the ErrPattern error.
func patternError(m, p string) *ServeMuxError {
	return &ServeMuxError{m, p, ErrPattern}
}

// duplicateError wraps the ErrDuplicate error.
func duplicateError(m, p string) *ServeMuxError {
	return &ServeMuxError{m, p, ErrDuplicate}
}

// notFoundError wraps the ErrNotFound error.
func notFoundError(m, p string) *ServeMuxError {
	return &ServeMuxError{m, p, ErrNotFound}
}

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
func (mux *ServeMux) insert(parts []string) (map[string]http.Handler, error) {
	cp := mux.tree.deepcopy()
	curr := cp.root

	for _, part := range parts {
		in := new(node)

		switch part[:1] {
		case pathToken:
			in.tid = slash
		case typeToken:
			conv := mux.converters[part[1:]]

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

	*mux.tree = *cp

	return curr.Methods, nil
}
