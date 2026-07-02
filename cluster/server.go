// Copyright (c) TFG Co. All Rights Reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package cluster

import (
	"encoding/json"
	"os"
	"sync/atomic"

	"github.com/topfreegames/pitaya/v2/logger"
)

const (
	StateActive       int32 = 0
	StateShuttingDown int32 = 1
)

// Server struct
type Server struct {
	loopbackEnabled bool
	state           atomic.Int32

	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Metadata map[string]string `json:"metadata"`
	Frontend bool              `json:"frontend"`
	Hostname string            `json:"hostname"`
}

// serverDTO is the JSON representation of Server, used for marshal/unmarshal.
type serverDTO struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Metadata map[string]string `json:"metadata"`
	Frontend bool              `json:"frontend"`
	Hostname string            `json:"hostname"`
	State    int32             `json:"state"`
}

type ServerOption func(*Server)

// NewServer creates a server with the given id, type, frontend flag and options.
func NewServer(id, serverType string, frontend bool, opts ...ServerOption) *Server {
	h, err := os.Hostname()
	if err != nil {
		logger.Log.Errorf("failed to get hostname: %s", err.Error())
	}
	s := &Server{
		ID:       id,
		Type:     serverType,
		Metadata: make(map[string]string),
		Frontend: frontend,
		Hostname: h,
	}
	s.state.Store(StateActive)
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithMetadata(metadata map[string]string) ServerOption {
	return func(s *Server) {
		if metadata != nil {
			s.Metadata = metadata
		}
	}
}

func WithLoopbackEnabled(enabled bool) ServerOption {
	return func(s *Server) {
		s.loopbackEnabled = enabled
	}
}

// AsJSONString returns the server as a json string
func (s *Server) AsJSONString() string {
	str, err := json.Marshal(s)
	if err != nil {
		logger.Log.Errorf("error getting server as json: %s", err.Error())
		return ""
	}
	return string(str)
}

// MarshalJSON implements json.Marshaler, atomically reading state.
func (s *Server) MarshalJSON() ([]byte, error) {
	return json.Marshal(&serverDTO{
		ID:       s.ID,
		Type:     s.Type,
		Metadata: s.Metadata,
		Frontend: s.Frontend,
		Hostname: s.Hostname,
		State:    s.state.Load(),
	})
}

// UnmarshalJSON implements json.Unmarshaler, atomically writing state.
func (s *Server) UnmarshalJSON(data []byte) error {
	var aux serverDTO
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	s.ID = aux.ID
	s.Type = aux.Type
	s.Metadata = aux.Metadata
	s.Frontend = aux.Frontend
	s.Hostname = aux.Hostname
	s.state.Store(aux.State)
	return nil
}

func (s *Server) IsLoopbackEnabled() bool {
	return s.loopbackEnabled
}

// GetState atomically returns the server state
func (s *Server) GetState() int32 {
	return s.state.Load()
}

// setState atomically sets the server state.
// Returns true if the state was changed, false if already in the given state.
func (s *Server) setState(state int32) {
	s.state.Store(state)
}
