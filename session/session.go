// Copyright (c) nano Author and TFG Co. All Rights Reserved.
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

package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/topfreegames/pitaya/v2/constants"
	"github.com/topfreegames/pitaya/v2/logger"
	"github.com/topfreegames/pitaya/v2/networkentity"
	"github.com/topfreegames/pitaya/v2/protos"
	"google.golang.org/protobuf/proto"
)

type sessionPoolImpl struct {
	sessionBindCallbacks []func(ctx context.Context, s Session) error
	afterBindCallbacks   []func(ctx context.Context, s Session) error
	handshakeValidators  map[string]func(data *HandshakeData) error

	// SessionCloseCallbacks contains global session close callbacks
	SessionCloseCallbacks []func(s Session)
	sessionsByUID         sync.Map
	sessionsByID          sync.Map
	sessionIDSvc          *sessionIDService
	// SessionCount keeps the current number of sessions
	SessionCount int64
}

// SessionPool centralizes all sessions within a Pitaya app
type SessionPool interface {
	NewSession(entity networkentity.NetworkEntity, frontend bool, UID ...string) Session
	GetSessionCount() int64
	GetSessionCloseCallbacks() []func(s Session)
	GetSessionByUID(uid string) Session
	GetSessionByID(id int64) Session
	OnSessionBind(f func(ctx context.Context, s Session) error)
	OnAfterSessionBind(f func(ctx context.Context, s Session) error)
	OnSessionClose(f func(s Session))
	CloseAll()
	AddHandshakeValidator(name string, f func(data *HandshakeData) error)
	GetNumberOfConnectedClients() int64
}

// HandshakeClientData represents information about the client sent on the handshake.
type HandshakeClientData struct {
	Platform    string `json:"platform"`
	LibVersion  string `json:"libVersion"`
	BuildNumber string `json:"clientBuildNumber"`
	Version     string `json:"clientVersion"`
}

// HandshakeData represents information about the handshake sent by the client.
// `sys` corresponds to information independent from the app and `user` information
// that depends on the app and is customized by the user.
type HandshakeData struct {
	Sys  HandshakeClientData    `json:"sys"`
	User map[string]interface{} `json:"user,omitempty"`
}

type sessionImpl struct {
	sync.RWMutex                                              // protect data
	id                  int64                                 // session global unique id
	uid                 string                                // binding user id
	lastTime            int64                                 // last heartbeat time
	entity              networkentity.NetworkEntity           // low-level network entity
	data                map[string]interface{}                // session data store
	handshakeData       *HandshakeData                        // handshake data received by the client
	handshakeValidators map[string]func(*HandshakeData) error // validations to run on handshake
	encodedData         []byte                                // session data encoded as a byte array
	OnCloseCallbacks    []func()                              //onClose callbacks
	IsFrontend          bool                                  // if session is a frontend session
	frontendID          string                                // the id of the frontend that owns the session
	frontendSessionID   int64                                 // the id of the session on the frontend server
	Subscriptions       []*nats.Subscription                  // subscription created on bind when using nats rpc server
	requestsInFlight    ReqInFlight                           // whether the session is waiting from a response from a remote
	pool                *sessionPoolImpl
}

type ReqInFlight struct {
	m  map[string]string
	mu sync.RWMutex
}

// Session represents a client session, which can store data during the connection.
// All data is released when the low-level connection is broken.
// Session instance related to the client will be passed to Handler method in the
// context parameter.
type Session interface {
	GetOnCloseCallbacks() []func()
	GetIsFrontend() bool
	GetSubscriptions() []*nats.Subscription
	SetOnCloseCallbacks(callbacks []func())
	SetIsFrontend(isFrontend bool)
	SetSubscriptions(subscriptions []*nats.Subscription)
	HasRequestsInFlight() bool
	GetRequestsInFlight() ReqInFlight
	SetRequestInFlight(reqID string, reqData string, inFlight bool)

	Push(route string, v interface{}) error
	ResponseMID(ctx context.Context, mid uint, v interface{}, err ...bool) error
	ID() int64
	UID() string
	FrontendID() string
	GetData() map[string]interface{}
	SetData(data map[string]interface{}) error
	GetDataEncoded() []byte
	SetDataEncoded(encodedData []byte) error
	SetFrontendData(frontendID string, frontendSessionID int64)
	Bind(ctx context.Context, uid string) error
	Kick(ctx context.Context) error
	OnClose(c func()) error
	Close()
	RemoteAddr() net.Addr
	Remove(key string) error
	Set(key string, value interface{}) error
	HasKey(key string) bool
	Get(key string) interface{}
	Int(key string) int
	Int8(key string) int8
	Int16(key string) int16
	Int32(key string) int32
	Int64(key string) int64
	Uint(key string) uint
	Uint8(key string) uint8
	Uint16(key string) uint16
	Uint32(key string) uint32
	Uint64(key string) uint64
	Float32(key string) float32
	Float64(key string) float64
	String(key string) string
	Value(key string) interface{}
	PushToFront(ctx context.Context) error
	Clear()
	SetHandshakeData(data *HandshakeData)
	GetHandshakeData() *HandshakeData
	ValidateHandshake(data *HandshakeData) error
	GetHandshakeValidators() map[string]func(data *HandshakeData) error
}

type sessionIDService struct {
	sid int64
}

func newSessionIDService() *sessionIDService {
	return &sessionIDService{
		sid: 0,
	}
}

// SessionID returns the session id
func (c *sessionIDService) sessionID() int64 {
	return atomic.AddInt64(&c.sid, 1)
}

// NewSession returns a new session instance
// a networkentity.NetworkEntity is a low-level network instance
func (pool *sessionPoolImpl) NewSession(entity networkentity.NetworkEntity, frontend bool, UID ...string) Session {
	s := &sessionImpl{
		id:                  pool.sessionIDSvc.sessionID(),
		entity:              entity,
		data:                make(map[string]interface{}),
		handshakeData:       nil,
		handshakeValidators: pool.handshakeValidators,
		lastTime:            time.Now().Unix(),
		OnCloseCallbacks:    []func(){},
		IsFrontend:          frontend,
		pool:                pool,
		requestsInFlight:    ReqInFlight{m: make(map[string]string)},
	}
	if frontend {
		pool.sessionsByID.Store(s.id, s)
		atomic.AddInt64(&pool.SessionCount, 1)
	}
	if len(UID) > 0 {
		s.uid = UID[0]
	}
	return s
}

// NewSessionPool returns a new session pool instance
func NewSessionPool() SessionPool {
	return &sessionPoolImpl{
		sessionBindCallbacks:  make([]func(ctx context.Context, s Session) error, 0),
		afterBindCallbacks:    make([]func(ctx context.Context, s Session) error, 0),
		handshakeValidators:   make(map[string]func(data *HandshakeData) error, 0),
		SessionCloseCallbacks: make([]func(s Session), 0),
		sessionIDSvc:          newSessionIDService(),
	}
}

func (pool *sessionPoolImpl) GetSessionCount() int64 {
	return pool.SessionCount
}

func (pool *sessionPoolImpl) GetSessionCloseCallbacks() []func(s Session) {
	return pool.SessionCloseCallbacks
}

// GetSessionByUID return a session bound to an user id
func (pool *sessionPoolImpl) GetSessionByUID(uid string) Session {
	// TODO: Block this operation in backend servers
	if val, ok := pool.sessionsByUID.Load(uid); ok {
		return val.(Session)
	}
	return nil
}

// GetSessionByID return a session bound to a frontend server id
func (pool *sessionPoolImpl) GetSessionByID(id int64) Session {
	// TODO: Block this operation in backend servers
	if val, ok := pool.sessionsByID.Load(id); ok {
		return val.(Session)
	}
	return nil
}

// OnSessionBind adds a method to be called when a session is bound
// same function cannot be added twice!
func (pool *sessionPoolImpl) OnSessionBind(f func(ctx context.Context, s Session) error) {
	// Prevents the same function to be added twice in onSessionBind
	sf1 := reflect.ValueOf(f)
	for _, fun := range pool.sessionBindCallbacks {
		sf2 := reflect.ValueOf(fun)
		if sf1.Pointer() == sf2.Pointer() {
			return
		}
	}
	pool.sessionBindCallbacks = append(pool.sessionBindCallbacks, f)
}

// OnAfterSessionBind adds a method to be called when session is bound and after all sessionBind callbacks
func (pool *sessionPoolImpl) OnAfterSessionBind(f func(ctx context.Context, s Session) error) {
	// Prevents the same function to be added twice in onSessionBind
	sf1 := reflect.ValueOf(f)
	for _, fun := range pool.afterBindCallbacks {
		sf2 := reflect.ValueOf(fun)
		if sf1.Pointer() == sf2.Pointer() {
			return
		}
	}
	pool.afterBindCallbacks = append(pool.afterBindCallbacks, f)
}

// OnSessionClose adds a method that will be called when every session closes
func (pool *sessionPoolImpl) OnSessionClose(f func(s Session)) {
	sf1 := reflect.ValueOf(f)
	for _, fun := range pool.SessionCloseCallbacks {
		sf2 := reflect.ValueOf(fun)
		if sf1.Pointer() == sf2.Pointer() {
			return
		}
	}
	pool.SessionCloseCallbacks = append(pool.SessionCloseCallbacks, f)
}

// CloseAll calls Close on all sessions
func (pool *sessionPoolImpl) CloseAll() {
	logger.Log.Infof("closing all sessions, %d sessions", pool.SessionCount)
	for pool.SessionCount > 0 {
		pool.sessionsByID.Range(func(_, value interface{}) bool {
			s := value.(Session)
			if s.HasRequestsInFlight() {
				reqsInFlight := s.GetRequestsInFlight()
				reqsInFlight.mu.RLock()
				for _, route := range reqsInFlight.m {
					logger.Log.Debugf("Session for user %s is waiting on a response for route %s from a remote server. Delaying session close.", s.UID(), route)
				}
				reqsInFlight.mu.RUnlock()
				return false
			} else {
				s.Close()
				return true
			}
		})
		logger.Log.Debugf("%d sessions remaining", pool.SessionCount)
		if pool.SessionCount > 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	logger.Log.Info("finished closing sessions")
}

// AddHandshakeValidator allows adds validation functions that will run when
// handshake packets are processed. Errors will be raised with the given name.
func (pool *sessionPoolImpl) AddHandshakeValidator(name string, f func(data *HandshakeData) error) {
	pool.handshakeValidators[name] = f
}

// GetNumberOfConnectedClients returns the number of connected clients
func (pool *sessionPoolImpl) GetNumberOfConnectedClients() int64 {
	return pool.GetSessionCount()
}

func (s *sessionImpl) updateEncodedData() error {
	var b []byte
	b, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	s.encodedData = b
	return nil
}

// GetOnCloseCallbacks ...
func (s *sessionImpl) GetOnCloseCallbacks() []func() {
	return s.OnCloseCallbacks
}

// GetIsFrontend ...
func (s *sessionImpl) GetIsFrontend() bool {
	return s.IsFrontend
}

// GetSubscriptions ...
func (s *sessionImpl) GetSubscriptions() []*nats.Subscription {
	return s.Subscriptions
}

// SetOnCloseCallbacks ...
func (s *sessionImpl) SetOnCloseCallbacks(callbacks []func()) {
	s.OnCloseCallbacks = callbacks
}

// SetIsFrontend ...
func (s *sessionImpl) SetIsFrontend(isFrontend bool) {
	s.IsFrontend = isFrontend
}

// SetSubscriptions ...
func (s *sessionImpl) SetSubscriptions(subscriptions []*nats.Subscription) {
	s.Subscriptions = subscriptions
}

// Push message to client
func (s *sessionImpl) Push(route string, v interface{}) error {
	return s.entity.Push(route, v)
}

// ResponseMID responses message to client, mid is
// request message ID
func (s *sessionImpl) ResponseMID(ctx context.Context, mid uint, v interface{}, err ...bool) error {
	return s.entity.ResponseMID(ctx, mid, v, err...)
}

// ID returns the session id
func (s *sessionImpl) ID() int64 {
	return s.id
}

// UID returns uid that bind to current session
func (s *sessionImpl) UID() string {
	return s.uid
}

// FrontendID returns the frontend id
func (a *sessionImpl) FrontendID() string {
	return a.frontendID
}

// GetData gets the data
func (s *sessionImpl) GetData() map[string]interface{} {
	s.RLock()
	defer s.RUnlock()

	return s.data
}

// SetData sets the whole session data
func (s *sessionImpl) SetData(data map[string]interface{}) error {
	s.Lock()
	defer s.Unlock()

	s.data = data
	return s.updateEncodedData()
}

// GetDataEncoded returns the session data as an encoded value
func (s *sessionImpl) GetDataEncoded() []byte {
	return s.encodedData
}

// SetDataEncoded sets the whole session data from an encoded value
func (s *sessionImpl) SetDataEncoded(encodedData []byte) error {
	if len(encodedData) == 0 {
		return nil
	}
	var data map[string]interface{}
	err := json.Unmarshal(encodedData, &data)
	if err != nil {
		return err
	}
	return s.SetData(data)
}

// SetFrontendData sets frontend id and session id
func (s *sessionImpl) SetFrontendData(frontendID string, frontendSessionID int64) {
	s.frontendID = frontendID
	s.frontendSessionID = frontendSessionID
}

// Bind bind UID to current session
func (s *sessionImpl) Bind(ctx context.Context, uid string) error {
	if uid == "" {
		return constants.ErrIllegalUID
	}

	if s.UID() != "" {
		return constants.ErrSessionAlreadyBound
	}

	s.uid = uid
	for _, cb := range s.pool.sessionBindCallbacks {
		err := cb(ctx, s)
		if err != nil {
			s.uid = ""
			return err
		}
	}

	// if code running on frontend server
	if s.IsFrontend {
		// If a session with the same UID already exists in this frontend server, close it
		if val, ok := s.pool.sessionsByUID.Load(uid); ok {
			val.(Session).Close()
		}
		s.pool.sessionsByUID.Store(uid, s)
	} else {
		// If frontentID is set this means it is a remote call and the current server
		// is not the frontend server that received the user request
		err := s.bindInFront(ctx)
		if err != nil {
			logger.Log.Error("error while trying to push session to front: ", err)
			s.uid = ""
			return err
		}
	}

	// invoke after callbacks on session bound
	for _, cb := range s.pool.afterBindCallbacks {
		err := cb(ctx, s)
		if err != nil {
			s.uid = ""
			return err
		}
	}

	return nil
}

// Kick kicks the user
func (s *sessionImpl) Kick(ctx context.Context) error {
	err := s.entity.Kick(ctx)
	if err != nil {
		return err
	}

	s.Close()
	return nil
}

// OnClose adds the function it receives to the callbacks that will be called
// when the session is closed
func (s *sessionImpl) OnClose(c func()) error {
	if !s.IsFrontend {
		return constants.ErrOnCloseBackend
	}
	s.OnCloseCallbacks = append(s.OnCloseCallbacks, c)
	return nil
}

// Close terminates current session, session related data will not be released,
// all related data should be cleared explicitly in Session closed callback
func (s *sessionImpl) Close() {
	if _, ok := s.pool.sessionsByID.LoadAndDelete(s.ID()); ok {
		atomic.AddInt64(&s.pool.SessionCount, -1)
	}
	// Only remove session by UID if the session ID matches the one being closed. This avoids problems with removing a valid session after the user has already reconnected before this session's heartbeat times out
	if val, ok := s.pool.sessionsByUID.Load(s.UID()); ok {
		if (val.(Session)).ID() == s.ID() {
			s.pool.sessionsByUID.Delete(s.UID())
		}
	}
	// TODO: this logic should be moved to nats rpc server
	if s.IsFrontend && s.Subscriptions != nil && len(s.Subscriptions) > 0 {
		// if the user is bound to an userid and nats rpc server is being used we need to unsubscribe
		for _, sub := range s.Subscriptions {
			err := sub.Drain()
			if err != nil {
				logger.Log.Errorf("error unsubscribing to user's messages channel: %s, this can cause performance and leak issues", err.Error())
			} else {
				logger.Log.Debugf("successfully unsubscribed to user's %s messages channel", s.UID())
			}
		}
	}
	s.entity.Close()
}

// RemoteAddr returns the remote network address.
func (s *sessionImpl) RemoteAddr() net.Addr {
	return s.entity.RemoteAddr()
}

// Remove delete data associated with the key from session storage
func (s *sessionImpl) Remove(key string) error {
	s.Lock()
	defer s.Unlock()

	delete(s.data, key)
	return s.updateEncodedData()
}

// Set associates value with the key in session storage
func (s *sessionImpl) Set(key string, value interface{}) error {
	s.Lock()
	defer s.Unlock()

	s.data[key] = value
	return s.updateEncodedData()
}

// HasKey decides whether a key has associated value
func (s *sessionImpl) HasKey(key string) bool {
	s.RLock()
	defer s.RUnlock()

	_, has := s.data[key]
	return has
}

// Get returns a key value
func (s *sessionImpl) Get(key string) interface{} {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return nil
	}
	return v
}

// Int returns the value associated with the key as a int.
func (s *sessionImpl) Int(key string) int {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return 0
	}

	value, ok := v.(int)
	if !ok {
		return 0
	}
	return value
}

// Int8 returns the value associated with the key as a int8.
func (s *sessionImpl) Int8(key string) int8 {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return 0
	}

	value, ok := v.(int8)
	if !ok {
		return 0
	}
	return value
}

// Int16 returns the value associated with the key as a int16.
func (s *sessionImpl) Int16(key string) int16 {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return 0
	}

	value, ok := v.(int16)
	if !ok {
		return 0
	}
	return value
}

// Int32 returns the value associated with the key as a int32.
func (s *sessionImpl) Int32(key string) int32 {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return 0
	}

	value, ok := v.(int32)
	if !ok {
		return 0
	}
	return value
}

// Int64 returns the value associated with the key as a int64.
func (s *sessionImpl) Int64(key string) int64 {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return 0
	}

	value, ok := v.(int64)
	if !ok {
		return 0
	}
	return value
}

// Uint returns the value associated with the key as a uint.
func (s *sessionImpl) Uint(key string) uint {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return 0
	}

	value, ok := v.(uint)
	if !ok {
		return 0
	}
	return value
}

// Uint8 returns the value associated with the key as a uint8.
func (s *sessionImpl) Uint8(key string) uint8 {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return 0
	}

	value, ok := v.(uint8)
	if !ok {
		return 0
	}
	return value
}

// Uint16 returns the value associated with the key as a uint16.
func (s *sessionImpl) Uint16(key string) uint16 {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return 0
	}

	value, ok := v.(uint16)
	if !ok {
		return 0
	}
	return value
}

// Uint32 returns the value associated with the key as a uint32.
func (s *sessionImpl) Uint32(key string) uint32 {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return 0
	}

	value, ok := v.(uint32)
	if !ok {
		return 0
	}
	return value
}

// Uint64 returns the value associated with the key as a uint64.
func (s *sessionImpl) Uint64(key string) uint64 {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return 0
	}

	value, ok := v.(uint64)
	if !ok {
		return 0
	}
	return value
}

// Float32 returns the value associated with the key as a float32.
func (s *sessionImpl) Float32(key string) float32 {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return 0
	}

	value, ok := v.(float32)
	if !ok {
		return 0
	}
	return value
}

// Float64 returns the value associated with the key as a float64.
func (s *sessionImpl) Float64(key string) float64 {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return 0
	}

	value, ok := v.(float64)
	if !ok {
		return 0
	}
	return value
}

// String returns the value associated with the key as a string.
func (s *sessionImpl) String(key string) string {
	s.RLock()
	defer s.RUnlock()

	v, ok := s.data[key]
	if !ok {
		return ""
	}

	value, ok := v.(string)
	if !ok {
		return ""
	}
	return value
}

// Value returns the value associated with the key as a interface{}.
func (s *sessionImpl) Value(key string) interface{} {
	s.RLock()
	defer s.RUnlock()

	return s.data[key]
}

func (s *sessionImpl) bindInFront(ctx context.Context) error {
	return s.sendRequestToFront(ctx, constants.SessionBindRoute, false)
}

// PushToFront updates the session in the frontend
func (s *sessionImpl) PushToFront(ctx context.Context) error {
	if s.IsFrontend {
		return constants.ErrFrontSessionCantPushToFront
	}
	return s.sendRequestToFront(ctx, constants.SessionPushRoute, true)
}

// Clear releases all data related to current session
func (s *sessionImpl) Clear() {
	s.Lock()
	defer s.Unlock()

	s.uid = ""
	s.data = map[string]interface{}{}
	s.updateEncodedData()
}

// SetHandshakeData sets the handshake data received by the client.
func (s *sessionImpl) SetHandshakeData(data *HandshakeData) {
	s.Lock()
	defer s.Unlock()

	s.handshakeData = data
}

// GetHandshakeData gets the handshake data received by the client.
func (s *sessionImpl) GetHandshakeData() *HandshakeData {
	return s.handshakeData
}

// GetHandshakeValidators return the handshake validators associated with the session.
func (s *sessionImpl) GetHandshakeValidators() map[string]func(data *HandshakeData) error {
	return s.handshakeValidators
}

func (s *sessionImpl) ValidateHandshake(data *HandshakeData) error {
	for name, fun := range s.handshakeValidators {
		if err := fun(data); err != nil {
			return fmt.Errorf("failed to run '%s' validator: %w. SessionId=%d", name, err, s.ID())
		}
	}

	return nil
}

func (s *sessionImpl) sendRequestToFront(ctx context.Context, route string, includeData bool) error {
	sessionData := &protos.Session{
		Id:  s.frontendSessionID,
		Uid: s.uid,
	}
	if includeData {
		sessionData.Data = s.encodedData
	}
	b, err := proto.Marshal(sessionData)
	if err != nil {
		return err
	}
	res, err := s.entity.SendRequest(ctx, s.frontendID, route, b)
	if err != nil {
		return err
	}
	logger.Log.Debugf("%s Got response: %+v", route, res)
	return nil
}

func (s *sessionImpl) HasRequestsInFlight() bool {
	return len(s.requestsInFlight.m) != 0
}

func (s *sessionImpl) GetRequestsInFlight() ReqInFlight {
	return s.requestsInFlight
}

func (s *sessionImpl) SetRequestInFlight(reqID string, reqData string, inFlight bool) {
	s.requestsInFlight.mu.Lock()
	if inFlight {
		s.requestsInFlight.m[reqID] = reqData
	} else {
		if _, ok := s.requestsInFlight.m[reqID]; ok {
			delete(s.requestsInFlight.m, reqID)
		}
	}
	s.requestsInFlight.mu.Unlock()
}
