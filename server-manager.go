package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

// Container struct
type ServerManager struct {
	Servers     []*Server
	ServerIndex int
	mutex       sync.Mutex
}

var (
	ServerPool ServerManager
)

// The workhorse function
func (s *ServerManager) Balance(w http.ResponseWriter, r *http.Request) {
	wasDead := false

	s.mutex.Lock()
	maxLen := len(s.Servers)

	// Make sure we have SOME servers registered
	if maxLen < 1 {
		// No servers registered, unlock early, send error status to client
		s.mutex.Unlock()
		http.Error(w, "No Services To Balance", http.StatusInternalServerError)

		return
	}

	// Set instance to next instance in line
	curr := s.Servers[s.ServerIndex%maxLen]

	// Before routing the user, make sure this instance isn't dead
	// and make sure that we haven't looped through all instances
	for tries := 0; curr.GetIsDead() && tries < maxLen; tries++ {
		// Since the instance was dead, keep progressing forward until one isn't dead

		// Flag ourselves for having entered this loop
		wasDead = true

		// Move to next server
		s.ServerIndex++
		curr = s.Servers[s.ServerIndex%maxLen]
	}

	// If we touched any dead instances, cull
	if wasDead {
		defer RemoveDeadServers()
	}

	// Get the redirect endpoint
	targetURL, err := url.Parse(s.Servers[s.ServerIndex%maxLen].URL)
	if err != nil {
		// Parse failed, print error and exit
		// Possibly remove this server?
		fmt.Println(err)
		return
	}

	// Set the next server up to operate on the next request
	s.ServerIndex++

	// Release our lock, we're done with the ServerManager
	s.mutex.Unlock()

	// Build our reverse proxy from what we've collected
	reverseProxy := httputil.NewSingleHostReverseProxy(targetURL)
	reverseProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		// We have failed to route our messages in some way
		// Print our error
		fmt.Println(e)

		// Check to make sure this isn't a timeout on our side - we want to ignore these
		if e.Error() != "context canceled" {
			// Not a timeout, go ahead and flag this endpoint as dead
			fmt.Printf("%s is dead\n", targetURL)
			curr.Dead(true)
		}
	}

	// Probably successful routing, print timestamp and where the client is headed
	fmt.Println(time.Now(), r.RequestURI)
	reverseProxy.ServeHTTP(w, r)
}

func RemoveDeadServers() {
	// Loop through all servers and remove those that are flagged
	for i := 0; i < len(ServerPool.Servers); i++ {
		if ServerPool.Servers[i].isDead {
			// Only lock our ServerPool if there happens to be a dead server
			ServerPool.mutex.Lock()

			// Copy the pointer to the last instance over this dead instance
			ServerPool.Servers[i] = ServerPool.Servers[len(ServerPool.Servers)-1]

			// Truncate to remove the now duplicate instance
			ServerPool.Servers = ServerPool.Servers[:len(ServerPool.Servers)-1]

			// Release our lock
			ServerPool.mutex.Unlock()

			// Lower i to not skip possibly new dead server at this index
			i--
		}
	}
}
