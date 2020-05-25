package mixer

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
)

type TestHandler string

func (TestHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}


type Assert struct {
	*testing.T
}

func (ass *Assert) integer(n1, n2 int, equal bool) bool {
	if equal {
		return n1 != n2
	}

	return n1 == n2
}

func (ass *Assert) IntEqual(n1, n2 int, msg string) {
	if ass.integer(n1, n2, true) {
		ass.Errorf("%s: integers are not equal (%d != %d)", msg, n1, n2)
	}
}

func (ass *Assert) IntNotEqual(n1, n2 int, msg string) {
	if ass.integer(n1, n2, false) {
		ass.Errorf("%s: integers are equal (%d == %d)", msg, n1, n2)
	}
}

func (ass *Assert) ptr(n1, n2 interface{}, equal bool) bool {
	ptr1 := reflect.ValueOf(n1).Pointer()
	ptr2 := reflect.ValueOf(n2).Pointer()

	if equal {
		return ptr1 != ptr2
	}

	return ptr1 == ptr2
}

func (ass *Assert) PtrEqual(n1, n2 interface{}, msg string) {
	if ass.ptr(n1, n2, true) {
		ass.Errorf("%s: pointers are not equal (%p != %p)", msg, n1, n2)
	}
}

func (ass *Assert) PtrNotEqual(n1, n2 interface{}, msg string) {
	if ass.ptr(n1, n2, false) {
		ass.Errorf("%s: pointers are equal (%p == %p)", msg, n1, n2)
	}
}

func (ass *Assert) Equal(n1, n2 interface{}, msg string) {
	if !reflect.DeepEqual(n1, n2) {
		ass.Errorf("%s: not equal (%v != %v)", msg, n1, n2)
	}
}

func (ass *Assert) EqualIndent(n1, n2 interface{}, msg, indent string) {
	if !reflect.DeepEqual(n1, n2) {
		ass.Errorf("%[1]s: not equal %[2]s%[3]s%[2]s%[4]s", msg, indent, n1, n2)
	}
}

func (t *tree) String() string {
	bytes, err := json.MarshalIndent(t.root, "", "\t")
	if err != nil {
		panic(err) // unexpected
	}
	return string(bytes)
}

func (n *node) String() string {
	bytes, err := json.MarshalIndent(n, "", "\t")
	if err != nil {
		panic(err) // unexpected
	}
	return string(bytes)
}

func TestSplitURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want []string
		err  error
	}{
		{
			name: "empty url",
			url:  "",
			want: []string{},
			err:  ErrPattern,
		},
		{
			name: "double root",
			url:  "//a/b",
			want: []string{"", "a", "b"},
			err:  ErrPattern,
		},
		{
			name: "double inner slash",
			url:  "/a//b",
			want: []string{"a", "", "b"},
			err:  ErrPattern,
		},
		{
			name: "double trailing slash",
			url:  "/a/b//",
			want: []string{"a", "b", "", "/"},
			err:  ErrPattern,
		},
		{
			name: "without root",
			url:  "a/b",
			want: []string{},
			err:  ErrPattern,
		},
		{
			name: "root only",
			url:  "/",
			want: []string{"/"},
			err:  nil,
		},
		{
			name: "with trailing slash",
			url:  "/a/b/",
			want: []string{"a", "b", "/"},
			err:  nil,
		},
		{
			name: "without trailing slash",
			url:  "/a/b",
			want: []string{"a", "b"},
			err:  nil,
		},
		{
			name: "long url",
			url:  "/a/b/c/d/e/f/",
			want: []string{"a", "b", "c", "d", "e", "f", "/"},
			err:  nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := splitURL(c.url)

			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("splitURL() got = %v, want = %v", got, c.want)
			}

			if !errors.Is(err, c.err) {
				t.Errorf("splitURL() error = %v, want = %v", err, c.err)
			}
		})
	}
}

func TestIntConv(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want int
		err  error
	}{
		{
			name: "valid",
			s:    "123",
			want: 123,
			err:  nil,
		},
		{
			name: "invalid",
			s:    "one-two-three",
			want: 0,
			err: &strconv.NumError{
				Func: "Atoi",
				Num:  "one-two-three",
				Err:  strconv.ErrSyntax,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := intConv(c.s)

			if got.(int) != c.want {
				t.Errorf("intConv() got = %v, want = %v", got, c.want)
			}

			if !reflect.DeepEqual(err, c.err) {
				t.Errorf("intConv() error = %v, want = %v", err, c.err)
			}
		})
	}
}

func TestStrConv(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want string
		err  error
	}{
		{
			name: "valid",
			s:    "one-two-three",
			want: "one-two-three",
			err:  nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := strConv(c.s)

			if got.(string) != c.want {
				t.Errorf("intConv() got = %v, want = %v", got, c.want)
			}

			if !reflect.DeepEqual(err, c.err) {
				t.Errorf("intConv() error = %v, want = %v", err, c.err)
			}
		})
	}
}

func TestTreeDeepcopy(t *testing.T) {
	assert := Assert{t}
	var ic convert = intConv
	var sc convert = strConv

	origin := &tree{
		root: &node{
			conv: &ic,
			Methods: map[string]http.Handler{
				"a": TestHandler("a"),
				"b": TestHandler("b"),
				"c": TestHandler("c"),
			},
			Children: map[string]*node{
				"ch1": {
					conv:     nil,
					Methods:  nil,
					Children: nil,
				},
				"ch2": {
					conv:    &sc,
					tid:     slash,
					Methods: nil,
					Children: map[string]*node{
						"sub-ch1": {
							conv: nil,
							tid:  param,
							Methods: map[string]http.Handler{
								"sub-a": TestHandler("sub-a"),
							},

							Children: map[string]*node{
								"sub-sub-ch1": {
									conv:     nil,
									Methods:  nil,
									Children: nil,
								},
								//"sub-sub-ch2": nil, // this is unexpected situation
							},
						},
					},
				},
			},
		},
	}
	copy_ := origin.deepcopy()

	assert.PtrNotEqual(origin, copy_, "tree")

	assert.PtrNotEqual(origin.root, copy_.root, "tree.root")
	assert.PtrEqual(origin.root.conv, copy_.root.conv, "tree.root.conv")
	assert.IntEqual(origin.root.tid, copy_.root.tid, "tree.root.tid")
	assert.PtrEqual(origin.root.Methods, copy_.root.Methods, "tree.root.Methods")
	assert.PtrNotEqual(origin.root.Children, copy_.root.Children, "tree.root.Children")

	ch1 := origin.root.Children["ch1"]
	ch1Copy_ := copy_.root.Children["ch1"]

	assert.PtrNotEqual(ch1, ch1Copy_, "tree.root.Children[ch1]")
	assert.PtrEqual(ch1.conv, ch1Copy_.conv, "tree.root.Children[ch1].conv")
	assert.IntEqual(ch1.tid, ch1Copy_.tid, "tree.root.Children[ch1].tid")
	assert.PtrEqual(ch1.Methods, ch1Copy_.Methods, "tree.root.Children[ch1].Methods")
	// because they are both empty
	assert.PtrEqual(ch1.Children, ch1Copy_.Children, "tree.root.Children[ch1].Children")

	ch2 := origin.root.Children["ch2"]
	ch2Copy_ := copy_.root.Children["ch2"]

	assert.PtrNotEqual(ch2, ch2Copy_, "tree.root.Children[ch2]")
	assert.PtrEqual(ch2.conv, ch2Copy_.conv, "tree.root.Children[ch2].conv")
	assert.IntEqual(ch2.tid, ch2Copy_.tid, "tree.root.Children[ch2].tid")
	assert.PtrEqual(ch2.Methods, ch2Copy_.Methods, "tree.root.Children[ch2].Methods")
	assert.PtrNotEqual(ch2.Children, ch2Copy_.Children, "tree.root.Children[ch2].Children")

	ch2Sub := ch2.Children["sub-ch1"]
	ch2SubCopy_ := ch2Copy_.Children["sub-ch1"]

	assert.PtrNotEqual(ch2Sub, ch2SubCopy_, "tree.root.Children[ch2][sub-ch1]")
	assert.PtrEqual(ch2Sub.conv, ch2SubCopy_.conv, "tree.root.Children[ch2][sub-ch1].conv")
	assert.IntEqual(ch2Sub.tid, ch2SubCopy_.tid, "tree.root.Children[ch2][sub-ch1].tid")
	assert.PtrEqual(ch2Sub.Methods, ch2SubCopy_.Methods, "tree.root.Children[ch2][sub-ch1].Methods")
	assert.PtrNotEqual(ch2Sub.Children, ch2SubCopy_.Children, "tree.root.Children[ch2][sub-ch1].Children")

	subSubCh1 := ch2Sub.Children["sub-sub-ch1"]
	subSubCh1Copy_ := ch2SubCopy_.Children["sub-sub-ch1"]
	assert.PtrNotEqual(subSubCh1, subSubCh1Copy_, "tree.root.Children[ch2][sub-ch1][sub-sub-ch1]")
	assert.PtrEqual(subSubCh1.conv, subSubCh1Copy_.conv, "tree.root.Children[ch2][sub-ch1][sub-sub-ch1].conv")
	assert.IntEqual(subSubCh1.tid, subSubCh1Copy_.tid, "tree.root.Children[ch2][sub-ch1][sub-sub-ch1].tid")
	assert.PtrEqual(subSubCh1.Methods, subSubCh1Copy_.Methods, "tree.root.Children[ch2][sub-ch1][sub-sub-ch1].Methods")
	assert.PtrEqual(subSubCh1.Children, subSubCh1Copy_.Children, "tree.root.Children[ch2][sub-ch1][sub-sub-ch1].Children")

	assert.PtrEqual(ch2.Children["sub-sub-ch2"], ch2Copy_.Children["sub-sub-ch2"], "last node")
}

func TestNodeFind(t *testing.T) {
	cases := []struct {
		name string
		tid  int
		want *node
	}{
		{
			name: "found",
			tid:  other,
			want: &node{tid: other},
		},
		{
			name: "not found",
			tid:  param,
			want: nil,
		},
	}

	n := &node{
		tid:      slash,
		Children: map[string]*node{"a": {tid: other}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := n.find(c.tid)

			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("node.find() got = %v, want = %v", got, c.want)
			}
		})
	}
}

func TestNodeInsert(t *testing.T) {
	var conv convert = intConv

	type args struct {
		key string
		in  *node
	}
	cases := []struct {
		name     string
		what     args
		want     bool
		whither  *node
		expected *node
	}{
		{
			name: "param typeID to empty node",
			what: args{"key", &node{
				conv: &conv,
				tid:  param,
			}},
			want:    true,
			whither: &node{},
			expected: &node{
				Children: map[string]*node{
					"key": {
						conv: &conv,
						tid:  param,
					},
				},
			},
		},
		{
			name: "other typeID to empty node",
			what: args{"key", &node{
				conv: &conv,
				tid:  other,
			}},
			want:    true,
			whither: &node{},
			expected: &node{
				Children: map[string]*node{
					"key": {
						conv: &conv,
						tid:  other,
					},
				},
			},
		},
		{
			name: "param typeID to other node",
			what: args{"key", &node{
				conv: &conv,
				tid:  param,
			}},
			want: false,
			whither: &node{
				Children: map[string]*node{
					"other": {
						tid: other,
					},
				},
			},
			expected: &node{
				Children: map[string]*node{
					"other": {
						tid: other,
					},
				},
			},
		},
		{
			name: "other typeID to param node",
			what: args{"key", &node{
				conv: &conv,
				tid:  other,
			}},
			want: false,
			whither: &node{
				Children: map[string]*node{
					"other": {
						conv: &conv,
						tid:  param,
					},
				},
			},
			expected: &node{
				Children: map[string]*node{
					"other": {
						conv: &conv,
						tid:  param,
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.whither.insert(c.what.key, c.what.in)

			if got != c.want {
				t.Errorf("node.insert() got = %v, want = %v", got, c.want)
			}

			if !reflect.DeepEqual(c.whither, c.expected) {
				t.Errorf("node.insert() struct = \n%s, want = \n%s", c.whither, c.expected)
			}
		})
	}
}

func TestServeMixerAddLogicCases(t *testing.T) {
	mix := NewServeMixer()
	exp := &tree{root: &node{tid: root}}
	ass := Assert{t}

	parts := []string{"a", "b"}
	exp.root.Children = map[string]*node{
		"a": {Children: map[string]*node{
			"b": {Methods: map[string]http.Handler{}},
		}},
	}
	_, err := mix.insert(parts)

	ass.Equal(err, nil, "without trailing slash")
	ass.EqualIndent(mix.tree, exp, "without trailing slash", "\n")

	parts = []string{"a", "b", "/"}
	exp.root.
		Children["a"].
		Children["b"].
		Children = map[string]*node{
		"/": {
			tid:     slash,
			Methods: map[string]http.Handler{},
		}}
	_, err = mix.insert(parts)

	ass.Equal(err, nil, "with trailing slash")
	ass.EqualIndent(mix.tree, exp, "with trailing slash", "\n")

	parts = []string{"a", "d"}
	exp.root.
		Children["a"].
		Children["d"] = &node{Methods: map[string]http.Handler{}}
	_, err = mix.insert(parts)

	ass.Equal(err, nil, "split paths")
	ass.EqualIndent(mix.tree, exp, "split paths", "\n")

	parts = []string{"a", "b", ":int"}
	exp.root.
		Children["a"].
		Children["b"].
		Children[":"] = &node{tid: param, conv: mix.converters["int"], Methods: map[string]http.Handler{},
	}
	_, err = mix.insert(parts)

	ass.Equal(err, nil, "typed path param")
	ass.EqualIndent(mix.tree, exp, "typed path param", "\n")

	_, err = mix.insert(parts)
	ass.Equal(err, nil, "duplicate typed path param")
	ass.EqualIndent(mix.tree, exp, "duplicate typed path param", "\n")

	parts = []string{"/"}
	exp.root.
		Children["/"] = &node{tid: slash, Methods: map[string]http.Handler{}}
	hm, err := mix.insert(parts)

	ass.Equal(err, nil, "add root")
	ass.EqualIndent(mix.tree, exp, "add root", "\n")

	hm[http.MethodGet] = TestHandler("/")
	exp.root.Children["/"].Methods[http.MethodGet] = TestHandler("/")
	hm, err = mix.insert(parts)

	ass.Equal(err, nil, "correct handler map")
	ass.EqualIndent(mix.tree, exp, "correct handler map", "\n")

	parts = []string{"a", "b", ":int", ":"}
	exp.root.
		Children["a"].
		Children["b"].
		Children[":"].
		Children = map[string]*node{
		":": {
			tid:     param,
			conv:    mix.converters[""],
			Methods: map[string]http.Handler{http.MethodPut: TestHandler("a/b/:int/:")},
		}}
	hm, err = mix.insert(parts)
	hm[http.MethodPut] = TestHandler("a/b/:int/:")

	ass.Equal(err, nil, "correct handler map and another converter")
	ass.EqualIndent(mix.tree, exp, "correct handler map and another converter", "\n")

	parts = []string{"a", ":int"}
	_, err = mix.insert(parts)

	ass.Equal(err, errors.New("multiple types for path param"), "different type (b vs. :int)")
	ass.EqualIndent(mix.tree, exp, "different type (b vs. :int)", "\n")

	parts = []string{"a", "b", ":str"}
	_, err = mix.insert(parts)

	ass.Equal(err, errors.New("multiple types for path param"), "different type (:int vs. :str)")
	ass.EqualIndent(mix.tree, exp, "different type (:int vs. :str)", "\n")

	parts = []string{"a", "b", "c"}
	_, err = mix.insert(parts)

	ass.Equal(err, errors.New("multiple types for path param"), "different type (:int vs. c)")
	ass.EqualIndent(mix.tree, exp, "different type (:int vs. c)", "\n")

	parts = []string{"a", "b", ":int", "/"}
	exp.root.
		Children["a"].
		Children["b"].
		Children[":"].
		Children["/"] = &node{tid: slash, Methods: map[string]http.Handler{}}
	_, err = mix.insert(parts)

	ass.Equal(err, nil, "different type (:int vs. /)")
	ass.EqualIndent(mix.tree, exp, "different type (:int vs. /)", "\n")

	parts = []string{"a", "b", ":int", ":str", "c"}
	exp.root.
		Children["a"].
		Children["b"].
		Children[":"].
		Children[":"].
		Children = map[string]*node{
		"c": {
			Methods: map[string]http.Handler{},
		}}
	_, err = mix.insert(parts)

	ass.Equal(err, nil, "invariant conv")
	ass.EqualIndent(mix.tree, exp, "invariant conv", "\n")

	parts = []string{"a", "b", ":int", ":", "/"}
	exp.root.
		Children["a"].
		Children["b"].
		Children[":"].
		Children[":"].
		Children["/"] = &node{tid: slash, Methods: map[string]http.Handler{}}
	_, err = mix.insert(parts)

	ass.Equal(err, nil, "invariant for /")
	ass.EqualIndent(mix.tree, exp, "invariant for /", "\n")

	parts = []string{"a", "b", ":int", ":", ":str"}
	_, err = mix.insert(parts)

	ass.Equal(err, errors.New("multiple types for path param"), "prevent /, c and : together")
	ass.EqualIndent(mix.tree, exp, "prevent /, c and : together", "\n")

	parts = []string{"a", "b", ":mem"}
	_, err = mix.insert(parts)

	ass.Equal(err, errors.New("invalid path param"), "invalid path param")
	ass.EqualIndent(mix.tree, exp, "invalid path param", "\n")

	parts = []string{"g", "g", "w", "p", ":gl"}
	_, err = mix.insert(parts)

	ass.Equal(err, errors.New("invalid path param"), "deep copy valid (new path)")
	ass.EqualIndent(mix.tree, exp, "deep copy valid (new path)", "\n")

	parts = []string{"a", "b", ":int", ":", "a", "b", ":hf"}
	_, err = mix.insert(parts)

	ass.Equal(err, errors.New("invalid path param"), "deep copy valid (exist path)")
	ass.EqualIndent(mix.tree, exp, "deep copy valid (exist path)", "\n")
}

func TestServeMixerAddDirectCases(t *testing.T) {
	mix := NewServeMixer()

	cases := []struct {
		name     string
		parts    []string
		root     *node
		wantRoot *node
		want     error
	}{
		{
			name:  "a vs. :int",
			parts: []string{":int"},
			root: &node{
				Children: map[string]*node{
					"a": {Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					"a": {Methods: map[string]http.Handler{}},
				},
			},
			want: errors.New("multiple types for path param"),
		},
		{
			name:  "a vs. /",
			parts: []string{"/"},
			root: &node{
				Children: map[string]*node{
					"a": {Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					"a": {Methods: map[string]http.Handler{}},
					"/": {tid: slash, Methods: map[string]http.Handler{}},
				},
			},
			want: nil,
		},
		{
			name:  ":int vs. a",
			parts: []string{"a"},
			root: &node{
				Children: map[string]*node{
					":": {tid: param, conv: mix.converters["int"], Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					":": {tid: param, conv: mix.converters["int"], Methods: map[string]http.Handler{}},
				},
			},
			want: errors.New("multiple types for path param"),
		},
		{
			name:  ":int vs. /",
			parts: []string{"/"},
			root: &node{
				Children: map[string]*node{
					":": {tid: param, conv: mix.converters["int"], Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					":": {tid: param, conv: mix.converters["int"], Methods: map[string]http.Handler{}},
					"/": {tid: slash, Methods: map[string]http.Handler{}},
				},
			},
			want: nil,
		},
		{
			name:  "/ vs. a",
			parts: []string{"a"},
			root: &node{
				Children: map[string]*node{
					"/": {tid: slash, Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					"/": {tid: slash, Methods: map[string]http.Handler{}},
					"a": {Methods: map[string]http.Handler{}},
				},
			},
			want: nil,
		},
		{
			name:  "/ vs. :str",
			parts: []string{":str"},
			root: &node{
				Children: map[string]*node{
					"/": {tid: slash, Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					"/": {tid: slash, Methods: map[string]http.Handler{}},
					":": {tid: param, conv: mix.converters["str"], Methods: map[string]http.Handler{}},
				},
			},
			want: nil,
		},
		{
			name:  "a vs. b",
			parts: []string{"b"},
			root: &node{
				Children: map[string]*node{
					"a": {Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					"a": {Methods: map[string]http.Handler{}},
					"b": {Methods: map[string]http.Handler{}},
				},
			},
			want: nil,
		},
		{
			name:  ":int vs. :",
			parts: []string{":"},
			root: &node{
				Children: map[string]*node{
					":": {tid: param, conv: mix.converters["int"], Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					":": {tid: param, conv: mix.converters["int"], Methods: map[string]http.Handler{}},
				},
			},
			want: errors.New("multiple types for path param"),
		},
		{
			name:  ": vs. :mem",
			parts: []string{":mem"},
			root: &node{
				Children: map[string]*node{
					":": {tid: param, conv: mix.converters[""], Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					":": {tid: param, conv: mix.converters[""], Methods: map[string]http.Handler{}},
				},
			},
			want: errors.New("invalid path param"),
		},
		{
			name:  ": vs. :str",
			parts: []string{":str"},
			root: &node{
				Children: map[string]*node{
					":": {tid: param, conv: mix.converters[""], Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					":": {tid: param, conv: mix.converters["str"], Methods: map[string]http.Handler{}},
				},
			},
			want: nil,
		},
		{
			name:  "/ and :int vs. a",
			parts: []string{"a"},
			root: &node{
				Children: map[string]*node{
					"/": {tid: slash, Methods: map[string]http.Handler{}},
					":": {tid: param, conv: mix.converters["int"], Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					"/": {tid: slash, Methods: map[string]http.Handler{}},
					":": {tid: param, conv: mix.converters["int"], Methods: map[string]http.Handler{}},
				},
			},
			want: errors.New("multiple types for path param"),
		},
		{
			name:  "/ and a vs. :int",
			parts: []string{":int"},
			root: &node{
				Children: map[string]*node{
					"/": {tid: slash, Methods: map[string]http.Handler{}},
					"a": {Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					"/": {tid: slash, Methods: map[string]http.Handler{}},
					"a": {Methods: map[string]http.Handler{}},
				},
			},
			want: errors.New("multiple types for path param"),
		},
		{
			name:  "add sub node",
			parts: []string{"b", "c"},
			root: &node{
				Children: map[string]*node{
					"/": {tid: slash, Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					"/": {tid: slash, Methods: map[string]http.Handler{}},
					"b": {
						Children: map[string]*node{
							"c": {Methods: map[string]http.Handler{}},
						},
					},
				},
			},
			want: nil,
		},
		{
			name:  "prevent create nodes if error was deeper",
			parts: []string{"a", "b", ":mem"},
			root: &node{
				Children: map[string]*node{
					"/": {tid: slash, Methods: map[string]http.Handler{}},
				},
			},
			wantRoot: &node{
				Children: map[string]*node{
					"/": {tid: slash, Methods: map[string]http.Handler{}},
				},
			},
			want: errors.New("invalid path param"),
		},
		//{
		//	name:     "first insert to empty root not create root node",
		//	parts:    []string{"a", "b", ":mem"},
		//	root:     nil,
		//	wantRoot: nil,
		//	want:     errors.New("invalid path param"),
		//},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mix.tree.root = c.root

			_, got := mix.insert(c.parts)

			ass := Assert{t}
			ass.Equal(got, c.want, "ServeMixer.add() error")
			ass.EqualIndent(mix.tree.root, c.wantRoot, "ServeMixer.add() tree", "\n")

			mix.tree.root = nil
		})
	}
}

func TestServeMixerHandle(t *testing.T) {
	mix := NewServeMixer()
	exp := NewServeMixer()
	ass := Assert{t}

	t.Run("panic if invalid method", func(t *testing.T) {
		defer func() {
			err := recover()
			if err == nil || errors.Unwrap(err.(error)) != ErrMethod {
				t.Errorf("mix.Handle() got = %v, want = %v", err, ErrMethod)
			}

			ass.EqualIndent(mix.tree, exp.tree, "mix.Handle() tree", "\n")
		}()

		mix.Handle(http.MethodConnect, "/a/b/c/", TestHandler("handler"))
	})

	t.Run("panic if nil handler", func(t *testing.T) {
		defer func() {
			err := recover()
			if err == nil || errors.Unwrap(err.(error)) != ErrHandler {
				t.Errorf("mix.Handle() got = %v, want = %v", err, ErrHandler)
			}

			ass.EqualIndent(mix.tree, exp.tree, "mix.Handle() tree", "\n")
		}()

		mix.Handle(http.MethodGet, "/a/b/c/", nil)
	})

	t.Run("panic if invalid pattern", func(t *testing.T) {
		defer func() {
			err := recover()
			if err == nil || errors.Unwrap(err.(error)) != ErrPattern {
				t.Errorf("mix.Handle() got = %v, want = %v", err, ErrPattern)
			}

			ass.EqualIndent(mix.tree, exp.tree, "mix.Handle() tree", "\n")
		}()

		mix.Handle(http.MethodGet, "/a/b//c/", TestHandler("handler"))
	})

	t.Run("panic if cannot add to tree", func(t *testing.T) {
		defer func() {
			err := recover()
			if err == nil || errors.Unwrap(err.(error)).Error() != "invalid path param" {
				t.Errorf("mix.Handle() got = %v, want = %v", err, "invalid path param")
			}

			ass.EqualIndent(mix.tree, exp.tree, "mix.Handle() tree", "\n")
		}()

		mix.Handle(http.MethodGet, "/a/:mem/", TestHandler("handler"))
	})

	mix.tree.root = &node{Children: map[string]*node{
		"/": {tid: slash, Methods: map[string]http.Handler{http.MethodGet: TestHandler("handler")}},
	}}
	exp.tree.root = &node{Children: map[string]*node{
		"/": {tid: slash, Methods: map[string]http.Handler{http.MethodGet: TestHandler("handler")}},
	}}

	t.Run("panic on duplicate handler", func(t *testing.T) {
		defer func() {
			err := recover()
			if err == nil || errors.Unwrap(err.(error)) != ErrDuplicate {
				t.Errorf("mix.Handle() got = %v, want = %v", err, ErrDuplicate)
			}

			ass.EqualIndent(mix.tree, exp.tree, "mix.Handle() tree", "\n")
		}()

		mix.Handle(http.MethodGet, "/", TestHandler("another handler"))
	})

	exp.tree.root.Children["/"].Methods[http.MethodPut] = TestHandler("another handler")

	t.Run("success add handler", func(t *testing.T) {
		mix.Handle(http.MethodPut, "/", TestHandler("another handler"))

		ass.EqualIndent(mix.tree, exp.tree, "mix.Handle() tree", "\n")
	})
}

func TestServeMixerHandleFunc(t *testing.T) {
	mix := NewServeMixer()
	exp := NewServeMixer()
	ass := Assert{t}

	t.Run("panic if nil handler", func(t *testing.T) {
		defer func() {
			err := recover()
			if err == nil || errors.Unwrap(err.(error)) != ErrHandler {
				t.Errorf("mix.HandleFunc() got = %v, want = %v", err, ErrHandler)
			}

			ass.EqualIndent(mix.tree, exp.tree, "mix.HandleFunc() tree", "\n")
		}()

		mix.HandleFunc(http.MethodGet, "/a/b/c/", nil)
	})

	fn := func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("good"))
	}

	t.Run("success add handler", func(t *testing.T) {
		mix.HandleFunc(http.MethodPut, "/", fn)

		if mix.tree.root.Children["/"].Methods[http.MethodPut] == nil {
			t.Errorf("mix.HandleFunc() does not add handler")
		}
	})
}

func TestNewServeMixer(t *testing.T) {
	mix := NewServeMixer()
	exp := &tree{root: &node{tid: root}}
	ass := Assert{t}

	ass.Equal(mix.tree, exp, "newServeMixer() tree")

	for _, name := range []string{"", "str"} {
		conv, ok := mix.converters[name]
		if !ok {
			t.Errorf("newServeMixer() converter %q not exist", name)
		}

		val, err := (*conv)("abc")
		if err != nil {
			t.Errorf("newServeMixer() converter %q unexpected error %v", name, err)
		}

		_, ok = val.(string)
		if !ok {
			t.Errorf("newServeMixer() converter %q wrong return type", name)
		}
	}

	name := "int"

	conv, ok := mix.converters[name]
	if !ok {
		t.Errorf("newServeMixer() converter %q not exist", name)
	}

	val, err := (*conv)("123")
	if err != nil {
		t.Errorf("newServeMixer() converter %q unexpected error %v", name, err)
	}

	_, ok = val.(int)
	if !ok {
		t.Errorf("newServeMixer() converter %q wrong return type", name)
	}
}

func mustReq(r *http.Request, err error) *http.Request {
	if err != nil {
		panic("unexpected error " + err.Error())
	}
	return r
}

func TestServeMixerHandlerLogicCases(t *testing.T) {
	mix := NewServeMixer()
	mix.tree.root = &node{Children: map[string]*node{
		"/": {
			tid: root,
			Methods: map[string]http.Handler{
				http.MethodGet: TestHandler("get"),
			},
		},
		"a": {
			Methods: map[string]http.Handler{
				http.MethodPut: TestHandler("put"),
			},
			Children: map[string]*node{
				":": {
					tid:  param,
					conv: mix.converters["int"],
					Methods: map[string]http.Handler{
						http.MethodPost:  TestHandler("post"),
						http.MethodPatch: TestHandler("patch"),
					},
					Children: map[string]*node{
						"/": {
							tid: slash,
							Methods: map[string]http.Handler{
								http.MethodGet: TestHandler("trailing slash"),
							},
						},
						"b": {
							Children: map[string]*node{
								":": {
									tid:  param,
									conv: mix.converters["str"],
									Methods: map[string]http.Handler{
										http.MethodDelete: TestHandler("delete"),
									},
								},
							},
						},
						"a": {
							Children: map[string]*node{
								"c": {
									Methods: map[string]http.Handler{
										http.MethodGet: TestHandler("last"),
									},
								},
							},
						},
					},
				},
			},
		},
	}}
	ass := Assert{t}

	req := mustReq(http.NewRequest(http.MethodGet, "/", nil))
	ctx := context.Background()
	got, err := mix.Handler(req)

	ass.Equal(got, TestHandler("get"), "for / got")
	ass.Equal(err, nil, "for / error")
	ass.Equal(req.Context(), ctx, "for / context")

	req = mustReq(http.NewRequest(http.MethodPut, "/", nil))
	got, err = mix.Handler(req)

	ass.Equal(got, nil, "non-exist handler got")
	ass.Equal(err, notFoundError(http.MethodPut, "/"), "non-exist handler error")
	ass.Equal(req.Context(), ctx, "non-exist handler context")

	req = mustReq(http.NewRequest(http.MethodPut, "/a", nil))
	got, err = mix.Handler(req)

	ass.Equal(got, TestHandler("put"), "for /a got")
	ass.Equal(err, nil, "for /a error")
	ass.Equal(req.Context(), ctx, "for /a context")

	req = mustReq(http.NewRequest(http.MethodPost, "/a/123", nil))
	ctx = context.WithValue(ctx, PathParamsCtxKey, PathParams{0: 123})
	got, err = mix.Handler(req)

	ass.Equal(got, TestHandler("post"), "for int got")
	ass.Equal(err, nil, "for int error")
	ass.Equal(req.Context(), ctx, "for int context")

	req = mustReq(http.NewRequest(http.MethodPost, "/a/one_two_three", nil))
	ctx = context.Background()
	got, err = mix.Handler(req)

	ass.Equal(got, nil, "for int wrong type got")
	ass.Equal(err, notFoundError(http.MethodPost, "/a/one_two_three"), "for int wrong type error")
	ass.Equal(req.Context(), ctx, "for int wrong type context")

	ctx = context.WithValue(context.Background(), "oldContext", "oldContext")
	req = mustReq(http.NewRequestWithContext(ctx, http.MethodPost, "/a/321", nil))
	ctx = context.WithValue(ctx, PathParamsCtxKey, PathParams{0: 321})
	got, err = mix.Handler(req)

	ass.Equal(got, TestHandler("post"), "save context for original request got")
	ass.Equal(err, nil, "save context for original request error")
	ass.Equal(req.Context(), ctx, "save context for original request context")

	req = mustReq(http.NewRequest(http.MethodPost, "/123", nil))
	ctx = context.Background()
	got, err = mix.Handler(req)

	ass.Equal(got, nil, "no handler for param got")
	ass.Equal(err, notFoundError(http.MethodPost, "/123"), "no handler for param error")
	ass.Equal(req.Context(), ctx, "no handler for param context")

	req = mustReq(http.NewRequest(http.MethodDelete, "/a/12/b/abc", nil))
	ctx = context.WithValue(ctx, PathParamsCtxKey, PathParams{0: 12, 1: "abc"})
	got, err = mix.Handler(req)

	ass.Equal(got, TestHandler("delete"), "multiple path params got")
	ass.Equal(err, nil, "multiple path params error")
	ass.Equal(req.Context(), ctx, "multiple path params context")

	mix = NewServeMixer()
	got, err = mix.Handler(req)

	ass.Equal(got, nil, "fresh mixer got")
	ass.Equal(err, notFoundError(http.MethodDelete, "/a/12/b/abc"), "fresh mixer error")
	ass.Equal(req.Context(), ctx, "fresh mixer context")
}

func TestGetPathParams(t *testing.T) {
	var exp PathParams // empty

	ctx := context.Background()
	req := mustReq(http.NewRequestWithContext(ctx, "", "", nil))
	ass := Assert{t}

	ass.Equal(GetPathParams(req), exp, "params not set")

	ctx = context.WithValue(ctx, "wrong key", PathParams{0: 12, 1: "abc"})
	req = mustReq(http.NewRequestWithContext(ctx, "", "", nil))

	ass.Equal(GetPathParams(req), exp, "wrong context key")

	ctx = context.WithValue(ctx, PathParamsCtxKey, PathParams{0: 12, 1: "abc"})
	req = mustReq(http.NewRequestWithContext(ctx, "", "", nil))
	exp = PathParams{0: 12, 1: "abc"}
	ass.Equal(GetPathParams(req), exp, "path params exist")
}

func mustResp(r *http.Response, err error) *http.Response {
	if err != nil {
		panic("unexpected error " + err.Error())
	}
	return r
}

func mustRead(b []byte, err error) string {
	if err != nil {
		panic("unexpected error " + err.Error())
	}
	return string(b)
}

func TestServeMixerServeHTTP(t *testing.T) {
	mix := NewServeMixer()
	mix.HandleFunc(http.MethodGet, "/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("done"))
	})

	ts := httptest.NewServer(mix)
	defer ts.Close()

	tc := ts.Client()

	respGood := mustResp(tc.Get(ts.URL))
	defer func() { _ = respGood.Body.Close() }()

	if mustRead(ioutil.ReadAll(respGood.Body)) != "done" {
		t.Errorf("asdasd")
	}

	respBad := mustResp(tc.Head(ts.URL))

	if respBad.StatusCode != http.StatusNotFound {
		t.Errorf("asdasd")
	}
}

func TestServeMixerError(t *testing.T) {
	err := ServeMixerError{
		method:  "method",
		pattern: "pattern",
		err:     errors.New("error"),
	}
	want := "mixer: handler (method) pattern error: error"

	if err.Error() != want {
		t.Errorf("ServeMixerError.Error() got = %s, want = %s", err.Error(), want)
	}

}
