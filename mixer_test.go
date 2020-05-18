package mixer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
)

func TestServeMixerSplitURL(t *testing.T) {
	cases := []struct {
		name, url string
		want      []string
	}{
		{
			name: "empty url",
			url:  "",
			want: nil,
		},
		{
			name: "double root",
			url:  "//a/b",
			want: nil,
		},
		{
			name: "double inner slash",
			url:  "/a//b",
			want: nil,
		},
		{
			name: "double trailing slash",
			url:  "/a/b//",
			want: nil,
		},
		{
			name: "without root",
			url:  "a/b",
			want: nil,
		},
		{
			name: "root only",
			url:  "/",
			want: []string{"/"},
		},
		{
			name: "with trailing slash",
			url:  "/a/b/",
			want: []string{"a", "b", "/"},
		},
		{
			name: "without trailing slash",
			url:  "/a/b",
			want: []string{"a", "b"},
		},
		{
			name: "long url",
			url:  "/a/b/c/d/e/f/",
			want: []string{"a", "b", "c", "d", "e", "f", "/"},
		},
	}

	mix := ServeMixer{}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mix.splitURL(c.url)

			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ServeMixer.splitURL() got = %v, want = %v", got, c.want)
			}
		})
	}
}

func (node *treeNode) String() string {
	bytes, err := json.MarshalIndent(node, "", "\t")
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

type TestHandler string

func (th TestHandler) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {}

type TestCaseAdd struct {
	*testing.T
	mix *ServeMixer
	exp *ServeMixer
}

func (tc *TestCaseAdd) assert(name string, got, want error) {
	if !reflect.DeepEqual(got, want) {
		tc.Errorf("%s: ServeMixer.add() got = %v, want = %v", name, got, want)
	}

	if !reflect.DeepEqual(tc.mix.root, tc.exp.root) {
		tc.Errorf("%s: ServeMixer.add() tree =\n%s, want =\n%s", name, tc.mix.root, tc.exp.root)
	}
}

func TestServeMixerAddLogicCases(t *testing.T) {
	mix := NewServeMixer()
	exp := NewServeMixer()
	tc := TestCaseAdd{T: t, mix: mix, exp: exp}

	parts := []string{"a", "b"}
	exp.root.Children = map[string]*treeNode{
		"a": {Children: map[string]*treeNode{
			"b": {Methods: handlerMap{}},
		}},
	}
	_, err := mix.add(parts)
	tc.assert("without trailing slash", err, nil)

	parts = []string{"a", "b", "/"}
	exp.root.
		Children["a"].
		Children["b"].
		Children = map[string]*treeNode{"/": {Methods: handlerMap{}}}
	_, err = mix.add(parts)
	tc.assert("with trailing slash", err, nil)

	parts = []string{"a", "d"}
	exp.root.
		Children["a"].
		Children["d"] = &treeNode{Methods: handlerMap{}}
	_, err = mix.add(parts)
	tc.assert("split paths", err, nil)

	parts = []string{"a", "b", ":int"}
	exp.root.
		Children["a"].
		Children["b"].Children[":"] = &treeNode{Methods: handlerMap{}, conv: mix.converters["int"]}
	_, err = mix.add(parts)
	tc.assert("typed path param", err, nil)

	_, err = mix.add(parts)
	tc.assert("duplicate typed path param", err, nil)

	parts = []string{"/"}
	exp.root.Children["/"] = &treeNode{Methods: handlerMap{}}
	hm, err := mix.add(parts)
	tc.assert("add root", err, nil)

	hm[http.MethodGet] = TestHandler("/")
	exp.root.Children["/"].Methods[http.MethodGet] = TestHandler("/")
	hm, err = mix.add(parts)
	tc.assert("correct handler map", err, nil)

	parts = []string{"a", "b", ":int", ":"}
	exp.root.
		Children["a"].
		Children["b"].
		Children[":"].
		Children = map[string]*treeNode{":": {Methods: handlerMap{http.MethodPut: TestHandler("a/b/:int/:")}, conv: mix.converters[""]}}
	hm, err = mix.add(parts)
	hm[http.MethodPut] = TestHandler("a/b/:int/:")
	tc.assert("correct handler map and another converter", err, nil)

	parts = []string{"a", ":int"}
	_, err = mix.add(parts)
	tc.assert("different type (b vs. :int)", err, errors.New("multiple types for path param"))

	parts = []string{"a", "b", ":str"}
	_, err = mix.add(parts)
	tc.assert("different type (:int vs. :str)", err, errors.New("multiple types for path param"))

	parts = []string{"a", "b", "c"}
	_, err = mix.add(parts)
	tc.assert("different type (:int vs. c)", err, errors.New("multiple types for path param"))

	parts = []string{"a", "b", ":int", "/"}
	exp.root.
		Children["a"].
		Children["b"].
		Children[":"].
		Children["/"] = &treeNode{Methods: handlerMap{}}
	_, err = mix.add(parts)
	tc.assert("different type (:int vs. /)", err, nil)

	parts = []string{"a", "b", ":int", ":str", "c"}
	exp.root.
		Children["a"].
		Children["b"].
		Children[":"].
		Children[":"].
		Children = map[string]*treeNode{"c": {Methods: handlerMap{}}}
	_, err = mix.add(parts)
	tc.assert("invariant conv", err, nil)

	parts = []string{"a", "b", ":int", ":", "/"}
	exp.root.
		Children["a"].
		Children["b"].
		Children[":"].
		Children[":"].
		Children["/"] = &treeNode{Methods: handlerMap{}}
	_, err = mix.add(parts)
	tc.assert("invariant for /", err, nil)

	parts = []string{"a", "b", ":int", ":", ":str"}
	_, err = mix.add(parts)
	tc.assert("prevent /, c and : together", err, errors.New("multiple types for path param"))

	parts = []string{"a", "b", ":mem"}
	_, err = mix.add(parts)
	tc.assert("invalid path param", err, errors.New("invalid path param"))

	parts = []string{"g", "g", "w", "p", ":gl"}
	_, err = mix.add(parts)
	tc.assert("deep copy valid (new path)", err, errors.New("invalid path param"))

	parts = []string{"a", "b", ":int", ":", "a", "b", ":hf"}
	_, err = mix.add(parts)
	tc.assert("deep copy valid (exist path)", err, errors.New("invalid path param"))
}

func TestServeMixerAddDirectCases(t *testing.T) {
	mix := NewServeMixer()

	cases := []struct {
		name     string
		parts    []string
		root     *treeNode
		wantRoot *treeNode
		want     error
	}{
		{
			name:  "a vs. :int",
			parts: []string{":int"},
			root: &treeNode{
				Children: map[string]*treeNode{
					"a": {Methods: handlerMap{}},
				},
			},
			wantRoot: &treeNode{
				Children: map[string]*treeNode{
					"a": {Methods: handlerMap{}},
				},
			},
			want: errors.New("multiple types for path param"),
		},
		{
			name:  "a vs. /",
			parts: []string{"/"},
			root: &treeNode{
				Children: map[string]*treeNode{
					"a": {Methods: handlerMap{}},
				},
			},
			wantRoot: &treeNode{
				Children: map[string]*treeNode{
					"a": {Methods: handlerMap{}},
					"/": {Methods: handlerMap{}},
				},
			},
			want: nil,
		},
		{
			name:  ":int vs. a",
			parts: []string{"a"},
			root: &treeNode{
				Children: map[string]*treeNode{
					":": {conv: mix.converters["int"], Methods: handlerMap{}},
				},
			},
			wantRoot: &treeNode{
				Children: map[string]*treeNode{
					":": {conv: mix.converters["int"], Methods: handlerMap{}},
				},
			},
			want: errors.New("multiple types for path param"),
		},
		{
			name:  ":int vs. /",
			parts: []string{"/"},
			root: &treeNode{
				Children: map[string]*treeNode{
					":": {conv: mix.converters["int"], Methods: handlerMap{}},
				},
			},
			wantRoot: &treeNode{
				Children: map[string]*treeNode{
					":": {conv: mix.converters["int"], Methods: handlerMap{}},
					"/": {Methods: handlerMap{}},
				},
			},
			want: nil,
		},
		{
			name:  "/ vs. a",
			parts: []string{"a"},
			root: &treeNode{
				Children: map[string]*treeNode{
					"/": {Methods: handlerMap{}},
				},
			},
			wantRoot: &treeNode{
				Children: map[string]*treeNode{
					"/": {Methods: handlerMap{}},
					"a": {Methods: handlerMap{}},
				},
			},
			want: nil,
		},
		{
			name:  "/ vs. :str",
			parts: []string{":str"},
			root: &treeNode{
				Children: map[string]*treeNode{
					"/": {Methods: handlerMap{}},
				},
			},
			wantRoot: &treeNode{
				Children: map[string]*treeNode{
					"/": {Methods: handlerMap{}},
					":": {conv: mix.converters["str"], Methods: handlerMap{}},
				},
			},
			want: nil,
		},
		{
			name:  "a vs. b",
			parts: []string{"b"},
			root: &treeNode{
				Children: map[string]*treeNode{
					"a": {Methods: handlerMap{}},
				},
			},
			wantRoot: &treeNode{
				Children: map[string]*treeNode{
					"a": {Methods: handlerMap{}},
					"b": {Methods: handlerMap{}},
				},
			},
			want: nil,
		},
		{
			name:  ":int vs. :",
			parts: []string{":"},
			root: &treeNode{
				Children: map[string]*treeNode{
					":": {conv: mix.converters["int"], Methods: handlerMap{}},
				},
			},
			wantRoot: &treeNode{
				Children: map[string]*treeNode{
					":": {conv: mix.converters["int"], Methods: handlerMap{}},
				},
			},
			want: errors.New("multiple types for path param"),
		},
		{
			name:  ": vs. :mem",
			parts: []string{":mem"},
			root: &treeNode{
				Children: map[string]*treeNode{
					":": {conv: mix.converters[""], Methods: handlerMap{}},
				},
			},
			wantRoot: &treeNode{
				Children: map[string]*treeNode{
					":": {conv: mix.converters[""], Methods: handlerMap{}},
				},
			},
			want: errors.New("invalid path param"),
		},
		{
			name:  ": vs. :str",
			parts: []string{":str"},
			root: &treeNode{
				Children: map[string]*treeNode{
					":": {conv: mix.converters[""], Methods: handlerMap{}},
				},
			},
			wantRoot: &treeNode{
				Children: map[string]*treeNode{
					":": {conv: mix.converters["str"], Methods: handlerMap{}},
				},
			},
			want: nil,
		},
		{
			name:  "/ and :int vs. a",
			parts: []string{"a"},
			root: &treeNode{
				Children: map[string]*treeNode{
					"/": {Methods: handlerMap{}},
					":": {conv: mix.converters["int"], Methods: handlerMap{}},
				},
			},
			wantRoot: &treeNode{
				Children: map[string]*treeNode{
					"/": {Methods: handlerMap{}},
					":": {conv: mix.converters["int"], Methods: handlerMap{}},
				},
			},
			want: errors.New("multiple types for path param"),
		},
		{
			name:  "/ and a vs. :int",
			parts: []string{":int"},
			root: &treeNode{
				Children: map[string]*treeNode{
					"/": {Methods: handlerMap{}},
					"a": {Methods: handlerMap{}},
				},
			},
			wantRoot: &treeNode{
				Children: map[string]*treeNode{
					"/": {Methods: handlerMap{}},
					"a": {Methods: handlerMap{}},
				},
			},
			want: errors.New("multiple types for path param"),
		},
		{
			name:  "add sub node",
			parts: []string{"b", "c"},
			root: &treeNode{
				Children: map[string]*treeNode{
					"/": {Methods: handlerMap{}},
				},
			},
			wantRoot: &treeNode{
				Children: map[string]*treeNode{
					"/": {Methods: handlerMap{}},
					"b": {
						Children: map[string]*treeNode{
							"c": {Methods: handlerMap{}},
						},
					},
				},
			},
			want: nil,
		},
		{
			name:  "prevent create nodes if error was deeper",
			parts: []string{"a", "b", ":mem"},
			root: &treeNode{Children: map[string]*treeNode{
				"/": {Methods: handlerMap{}},
			}},
			wantRoot: &treeNode{Children: map[string]*treeNode{
				"/": {Methods: handlerMap{}},
			}},
			want: errors.New("invalid path param"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mix.root = c.root

			_, got := mix.add(c.parts)

			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ServeMixer.add() got = %v, want = %v", got, c.want)
			}

			if !reflect.DeepEqual(mix.root, c.wantRoot) {
				t.Errorf("ServeMixer.add() tree =\n%s, want =\n%s", mix.root, c.wantRoot)
			}

			mix.root = nil
		})
	}
}

func TestServeMixerMethod(t *testing.T) {
	mix := NewServeMixer()
	exp := NewServeMixer()

	t.Run("panic if wrong method", func(t *testing.T) {
		defer func() {
			err := recover()
			if err == nil || errors.Unwrap(err.(error)) != ErrNotMethod {
				t.Errorf("mix.method() got = %v, want = %v", err, ErrNotMethod)
			}

			if !reflect.DeepEqual(mix.root, exp.root) {
				t.Errorf("mix.method() tree =\n%s, want =\n%s", mix.root, exp.root)
			}
		}()

		mix.method("/a/b/c/", http.MethodConnect, TestHandler("handler"))
	})

	t.Run("panic if invalid pattern", func(t *testing.T) {
		defer func() {
			err := recover()
			if err == nil || errors.Unwrap(err.(error)) != ErrNotPattern {
				t.Errorf("mix.method() got = %v, want = %v", err, ErrNotPattern)
			}

			if !reflect.DeepEqual(mix.root, exp.root) {
				t.Errorf("mix.method() tree =\n%s, want =\n%s", mix.root, exp.root)
			}
		}()

		mix.method("/a/b//c/", http.MethodGet, TestHandler("handler"))
	})

	t.Run("panic if cannot add to tree", func(t *testing.T) {
		defer func() {
			err := recover()
			if err == nil || errors.Unwrap(err.(error)).Error() != "invalid path param" {
				t.Errorf("mix.method() got = %v, want = %v", err, "invalid path param")
			}

			if !reflect.DeepEqual(mix.root, exp.root) {
				t.Errorf("mix.method() tree =\n%s, want =\n%s", mix.root, exp.root)
			}
		}()

		mix.method("/a/:mem/", http.MethodGet, TestHandler("handler"))
	})

	mix.root.Children = map[string]*treeNode{
		"/": {Methods: handlerMap{http.MethodGet: TestHandler("handler")}},
	}
	exp.root.Children = map[string]*treeNode{
		"/": {Methods: handlerMap{http.MethodGet: TestHandler("handler")}},
	}

	t.Run("panic on duplicate handler", func(t *testing.T) {
		defer func() {
			err := recover()
			if err == nil || errors.Unwrap(err.(error)) != ErrDuplicate {
				t.Errorf("mix.method() got = %v, want = %v", err, ErrDuplicate)
			}

			if !reflect.DeepEqual(mix.root, exp.root) {
				t.Errorf("mix.method() tree =\n%s, want =\n%s", mix.root, exp.root)
			}
		}()

		mix.method("/", http.MethodGet, TestHandler("another handler"))
	})

	exp.root.Children["/"].Methods[http.MethodPut] = TestHandler("another handler")

	t.Run("success add handler", func(t *testing.T) {
		mix.method("/", http.MethodPut, TestHandler("another handler"))

		if !reflect.DeepEqual(mix.root, exp.root) {
			t.Errorf("mix.method() tree =\n%s, want =\n%s", mix.root, exp.root)
		}
	})
}

type TestCaseHandler struct {
	*testing.T
	mix *ServeMixer
	req *http.Request
	ctx context.Context
}

func (tc *TestCaseHandler) assert(name string, want http.Handler, wantErr error) {
	got, gotErr := tc.mix.handler(tc.req)

	if !reflect.DeepEqual(got, want) {
		tc.Errorf("%s: ServeMixer.handler() got = %v, want = %v", name, got, want)
	}

	if !reflect.DeepEqual(gotErr, wantErr) {
		tc.Errorf("%s: ServeMixer.hanlder() error = %v, want = %v", name, gotErr, wantErr)
	}

	if !reflect.DeepEqual(tc.req.Context(), tc.ctx) {
		tc.Errorf("%s: ServeMixer.handler() context = %v, want = %v", name, tc.req.Context(), tc.ctx)
	}
}

func must(r *http.Request, err error) *http.Request {
	if err != nil {
		panic("unexpected request error " + err.Error())
	}
	return r
}

func TestServeMixerHandlerLogicCases(t *testing.T) {
	mix := NewServeMixer()
	mix.root.Children = map[string]*treeNode{
		"/": {
			Methods: handlerMap{
				http.MethodGet: TestHandler("get"),
			},
		},
		"a": {
			Methods: handlerMap{
				http.MethodPut: TestHandler("put"),
			},
			Children: map[string]*treeNode{
				":": {
					Methods: handlerMap{
						http.MethodPost:  TestHandler("post"),
						http.MethodPatch: TestHandler("patch"),
					},
					conv: mix.converters["int"],
					Children: map[string]*treeNode{
						"/": {
							Methods: handlerMap{
								http.MethodGet: TestHandler("trailing slash"),
							},
						},
						"b": {
							Children: map[string]*treeNode{
								":": {
									Methods: handlerMap{
										http.MethodDelete: TestHandler("delete"),
									},
									conv: mix.converters["str"],
								},
							},
						},
						"a": {
							Children: map[string]*treeNode{
								"c": {
									Methods: handlerMap{
										http.MethodGet: TestHandler("last"),
									},
								},
							},
						},
					},
				},
			},
		},
	}
	tc := TestCaseHandler{T: t, mix: mix}

	tc.req = must(http.NewRequest(http.MethodGet, "/", nil))
	tc.ctx = context.Background()
	tc.assert("for /", TestHandler("get"), nil)

	tc.req = must(http.NewRequest(http.MethodPut, "/", nil))
	tc.ctx = context.Background()
	tc.assert("non-exist handler", nil, ErrNotFound)

	tc.req = must(http.NewRequest(http.MethodPut, "/a", nil))
	tc.ctx = context.Background()
	tc.assert("for /a", TestHandler("put"), nil)

	tc.req = must(http.NewRequest(http.MethodPost, "/a/123", nil))
	tc.ctx = context.WithValue(context.Background(), PathParamsCtxKey, PathParams{0: 123})
	tc.assert("for int", TestHandler("post"), nil)

	tc.req = must(http.NewRequest(http.MethodPost, "/a/one_two_three", nil))
	tc.ctx = context.Background()
	tc.assert("for int wrong type", nil, ErrNotFound)

	ctx := context.WithValue(context.Background(), "oldContext", "oldContext")
	tc.req = must(http.NewRequestWithContext(ctx, http.MethodPost, "/a/321", nil))
	tc.ctx = context.WithValue(ctx, PathParamsCtxKey, PathParams{0: 321})
	tc.assert("save context for original request", TestHandler("post"), nil)

	tc.req = must(http.NewRequest(http.MethodPost, "/123", nil))
	tc.ctx = context.Background()
	tc.assert("no handler for param", nil, ErrNotFound)
}

func TestServeMixerGet(t *testing.T) {
	mix := NewServeMixer()
	exp := &treeNode{
		Children: map[string]*treeNode{
			"/": {
				Methods: handlerMap{
					http.MethodGet: TestHandler("get"),
				},
			},
		},
	}
	mix.Get("/", TestHandler("get"))

	if !reflect.DeepEqual(mix.root, exp) {
		t.Errorf("mix.Get() got = \n%s, want = \n%s", mix.root, exp)
	}
}

func TestServeMixerPost(t *testing.T) {
	mix := NewServeMixer()
	exp := &treeNode{
		Children: map[string]*treeNode{
			"/": {
				Methods: handlerMap{
					http.MethodPost: TestHandler("post"),
				},
			},
		},
	}
	mix.Post("/", TestHandler("post"))

	if !reflect.DeepEqual(mix.root, exp) {
		t.Errorf("mix.Post() got = \n%s, want = \n%s", mix.root, exp)
	}
}

func TestServeMixerPut(t *testing.T) {
	mix := NewServeMixer()
	exp := &treeNode{
		Children: map[string]*treeNode{
			"/": {
				Methods: handlerMap{
					http.MethodPut: TestHandler("put"),
				},
			},
		},
	}
	mix.Put("/", TestHandler("put"))

	if !reflect.DeepEqual(mix.root, exp) {
		t.Errorf("mix.Put() got = \n%s, want = \n%s", mix.root, exp)
	}
}

func TestServeMixerPatch(t *testing.T) {
	mix := NewServeMixer()
	exp := &treeNode{
		Children: map[string]*treeNode{
			"/": {
				Methods: handlerMap{
					http.MethodPatch: TestHandler("patch"),
				},
			},
		},
	}
	mix.Patch("/", TestHandler("patch"))

	if !reflect.DeepEqual(mix.root, exp) {
		t.Errorf("mix.Patch() got = \n%s, want = \n%s", mix.root, exp)
	}
}

func TestServeMixerDelete(t *testing.T) {
	mix := NewServeMixer()
	exp := &treeNode{
		Children: map[string]*treeNode{
			"/": {
				Methods: handlerMap{
					http.MethodDelete: TestHandler("delete"),
				},
			},
		},
	}
	mix.Delete("/", TestHandler("delete"))

	if !reflect.DeepEqual(mix.root, exp) {
		t.Errorf("mix.Delete() got = \n%s, want = \n%s", mix.root, exp)
	}
}

func TestNewServeMixer(t *testing.T) {
	mix := NewServeMixer()
	root := new(treeNode)

	if !reflect.DeepEqual(mix.root, root) {
		t.Errorf("NewServeMixer() got = %s, want = %s", mix.root, root)
	}

	for _, name := range []string{"", "str"} {
		conv, ok := mix.converters[name]
		if !ok {
			t.Errorf("NewServeMixer() converter %q not exist", name)
		}

		val, err := conv.fn("abc")
		if err != nil {
			t.Errorf("NewServeMixer() converter %q unexpected error %v", name, err)
		}

		_, ok = val.(string)
		if !ok {
			t.Errorf("NewServeMixer() converter %q wrong return type", name)
		}
	}

	name := "int"

	conv, ok := mix.converters[name]
	if !ok {
		t.Errorf("NewServeMixer() converter %q not exist", name)
	}

	val, err := conv.fn("123")
	if err != nil {
		t.Errorf("NewServeMixer() converter %q unexpected error %v", name, err)
	}

	_, ok = val.(int)
	if !ok {
		t.Errorf("NewServeMixer() converter %q wrong return type", name)
	}
}
