package notes

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type Generation struct {
	Token string `json:"token"`
	Done  bool   `json:"done,omitempty"`
	Error string `json:"error,omitempty"`
}

// Open uses an explicit store path inherited by a detached worker.
func Open(path string) *Store {
	s := &Store{path: path, m: map[string]string{}}
	s.Reload()
	return s
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Begin reserves the newest request before spawning any asynchronous work.
func (s *Store) Begin(id string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("notes unavailable")
	}
	token := rand.Text()
	err := s.update(func(_ map[string]string, requests map[string]Generation) {
		requests[id] = Generation{Token: token}
	})
	return token, err
}

// Cancel invalidates even an identical or cleared note (the ABA case).
func (s *Store) Cancel(id string) error {
	if s == nil {
		return nil
	}
	return s.update(func(_ map[string]string, requests map[string]Generation) {
		delete(requests, id)
	})
}

// Complete compares the durable token under the same lock as all note writes.
func (s *Store) Complete(id, token, title string, generationErr error) (bool, error) {
	applied := false
	err := s.update(func(stored map[string]string, requests map[string]Generation) {
		request, ok := requests[id]
		if !ok || request.Token != token || request.Done {
			return
		}
		request.Done = true
		if generationErr != nil {
			request.Error = generationErr.Error()
		} else {
			stored[id] = title
			applied = true
		}
		requests[id] = request
	})
	return applied && err == nil, err
}

func (s *Store) Generation(id string) Generation {
	if s == nil {
		return Generation{}
	}
	if s.path == "" {
		return s.requests[id]
	}
	requests, err := readGenerations(s.path)
	if err != nil {
		return Generation{}
	}
	return requests[id]
}

func readGenerations(path string) (map[string]Generation, error) {
	data, err := os.ReadFile(path + ".requests")
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Generation{}, nil
	}
	if err != nil {
		return nil, err
	}
	requests := map[string]Generation{}
	if err := json.Unmarshal(data, &requests); err != nil {
		return nil, err
	}
	if requests == nil {
		return nil, fmt.Errorf("invalid note requests")
	}
	return requests, nil
}
