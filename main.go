package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func Serve() {
	// Potentially expand to also supporting HTTPS?
	// Not sure that is necessary when we're doing no validation,
	// just passing on to whatever endpoint is registered
	s := http.Server{
		Addr:    ":80",
		Handler: http.HandlerFunc(ServerPool.Balance),
	}

	if err := s.ListenAndServe(); err != nil {
		// Explain the reason for closing before you go
		fmt.Println(err)
	}
}

func ListenForRegistration() {
	// I absolutely love chi routing, going to keep using it forever
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// The only endpoint we need, for everything else we will 404
	r.Post("/lb/new", func(w http.ResponseWriter, r *http.Request) {
		// Create a temporary instance
		server := &Server{}

		// The only thing we expect from the server attempting to register
		// is the url they want us to route to as a json string
		err := json.NewDecoder(r.Body).Decode(server)
		if err != nil {
			fmt.Println("Error adding new server", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Waiting until necessary to minimize time locked
		ServerPool.mutex.Lock()
		ServerPool.Servers = append(ServerPool.Servers, server)
		ServerPool.mutex.Unlock()

		fmt.Println("Added new server :", server)
	})

	http.ListenAndServe(":4041", r)
}

func main() {
	// A simple println to express the state we're in
	fmt.Println("Starting...")

	// Listens for servers to connect and present their connection information
	go ListenForRegistration()

	// Listen for clients to reach out and route them to live servers
	Serve()
}
