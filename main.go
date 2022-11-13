package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	URL    string `json:"url"`
	isDead bool
	mutex  sync.RWMutex
}

func (s *Server) Dead(b bool) {
	s.mutex.Lock()
	s.isDead = b
	s.mutex.Unlock()
}

func (s *Server) GetIsDead() bool {
	s.mutex.RLock()
	isDead := s.isDead
	s.mutex.RUnlock()

	return isDead
}

type ServerManager struct {
	Servers     []*Server
	ServerIndex int
	mutex       sync.Mutex
}

var (
	ServerPool ServerManager
)

func RemoveDeadServers() {
	for i := 0; i < len(ServerPool.Servers); i++ {
		if ServerPool.Servers[i].isDead {
			ServerPool.mutex.Lock()
			ServerPool.Servers[i] = ServerPool.Servers[len(ServerPool.Servers)-1]
			ServerPool.Servers = ServerPool.Servers[:len(ServerPool.Servers)-1]
			ServerPool.mutex.Unlock()

			i--
		}
	}
}

func (s *ServerManager) Balance(w http.ResponseWriter, r *http.Request) {
	wasDead := false

	s.mutex.Lock()
	maxLen := len(s.Servers)
	if maxLen < 1 {
		s.mutex.Unlock()
		http.Error(w, "No Services To Balance", http.StatusInternalServerError)
		return
	}

	curr := s.Servers[s.ServerIndex%maxLen]
	for curr.GetIsDead() {
		wasDead = true
		s.ServerIndex++
		curr = s.Servers[s.ServerIndex%maxLen]
	}

	// If we touched any dead instances, cull
	if wasDead {
		defer RemoveDeadServers()
	}

	targetURL, err := url.Parse(s.Servers[s.ServerIndex%maxLen].URL)
	if err != nil {
		fmt.Println(err)
		return
	}

	s.ServerIndex++
	s.mutex.Unlock()

	reverseProxy := httputil.NewSingleHostReverseProxy(targetURL)
	reverseProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		fmt.Println(e)
		if e.Error() != "context canceled" {
			fmt.Printf("%s is dead\n", targetURL)
			curr.Dead(true)
		}
	}

	fmt.Println(time.Now(), r.RequestURI)
	reverseProxy.ServeHTTP(w, r)
}

func Serve() {

	s := http.Server{
		Addr:    ":80",
		Handler: http.HandlerFunc(ServerPool.Balance),
	}

	if err := s.ListenAndServe(); err != nil {
		panic(err)
	}
}

func ListenForRegistration() {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/lb/new", func(w http.ResponseWriter, r *http.Request) {
		ServerPool.mutex.Lock()

		server := &Server{}

		err := json.NewDecoder(r.Body).Decode(server)
		if err != nil {
			fmt.Println("Error adding new server", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		ServerPool.Servers = append(ServerPool.Servers, server)
		fmt.Println("Added new server :", server)
		ServerPool.mutex.Unlock()
	})

	http.ListenAndServe(":4041", r)
}

func main() {
	fmt.Println("Starting...")
	go ListenForRegistration()
	Serve()
}
