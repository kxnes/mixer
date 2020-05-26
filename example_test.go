package mixer

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

type apiHandler struct{}

func (apiHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

func ExampleServeMuxHandle() {
	mux := New()
	mux.Handle(http.MethodGet, "/api/v1", apiHandler{})
	mux.HandleFunc(http.MethodGet, "/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "Welcome to the home page!")
	})
}

type Item struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type Catalog struct {
	mu    *sync.RWMutex
	items map[int]*Item
	last  int
}

func (c *Catalog) Get(id int) *Item {
	c.mu.RLock()
	i := c.items[id]
	c.mu.RUnlock()
	return i
}

func (c *Catalog) Add(name string) {
	c.mu.Lock()
	c.items[c.last] = &Item{ID: c.last, Name: name}
	c.last++
	c.mu.Unlock()
}

func (c *Catalog) All(w http.ResponseWriter, _ *http.Request) {
	c.mu.RLock()
	err := json.NewEncoder(w).Encode(c.items)
	c.mu.RUnlock()

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (c *Catalog) Create(w http.ResponseWriter, r *http.Request) {
	name := r.PostFormValue("name")

	c.mu.Lock()
	c.last++
	c.items[c.last] = &Item{ID: c.last, Name: name}
	c.mu.Unlock()

	w.WriteHeader(http.StatusCreated)
}

func (c *Catalog) Retrieve(w http.ResponseWriter, r *http.Request) {
	id := GetPathParams(r)[0].(int)

	c.mu.RLock()
	i := c.items[id]
	c.mu.RUnlock()

	if i == nil {
		i = &Item{}
	}

	err := json.NewEncoder(w).Encode(i)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func ExampleServeMuxUsage() {
	catalog := Catalog{
		mu: &sync.RWMutex{},
		items: map[int]*Item{
			1: {ID: 1, Name: "Foo"},
			2: {ID: 2, Name: "Bar"},
		},
		last: 2,
	}

	mux := New()

	mux.GetFunc("/catalog/", catalog.All)
	mux.PostFunc("/catalog/", catalog.Create)
	mux.GetFunc("/catalog/:int", catalog.Retrieve)

	// taste it!
	//
	// $ curl http://127.0.0.1:8080/catalog
	// # 404 page not found
	//
	// $ curl http://127.0.0.1:8080/catalog/
	// # {"1":{"id":1,"name":"Foo"},"2":{"id":2,"name":"Bar"}}
	//
	// $ curl -X POST -d "name=Mem" http://127.0.0.1:8080/catalog/
	//
	// $ curl http://127.0.0.1:8080/catalog/
	// # {"1":{"id":1,"name":"Foo"},"2":{"id":2,"name":"Bar"},"3":{"id":3,"name":"Mem"}}
	//
	// $ curl http://127.0.0.1:8080/catalog/3
	// # {"id":3,"name":"Mem"}
	//
	// $ curl http://127.0.0.1:8080/catalog/4
	// # {}
	//
	// $ curl -X POST http://127.0.0.1:8080/catalog/4
	// # 404 page not found

	log.Fatalln(http.ListenAndServe(":8080", mux))
}
