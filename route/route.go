// Copyright (c) TFG Co and nano Authors. All Rights Reserved.
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

package route

import (
	"errors"
	"fmt"
	"strings"

	"github.com/topfreegames/pitaya/v2/logger"
)

var (
	// ErrRouteFieldCantEmpty error
	ErrRouteFieldCantEmpty = errors.New("route field can not be empty")
	// ErrInvalidRoute error
	ErrInvalidRoute = errors.New("invalid route")
)

// Route struct
type Route struct {
	SvType     string
	Service    string
	Method     string
	ServiceOri string
	Instance   string
}

// NewRoute creates a new route
func NewRoute(server, service, method string) *Route {
	var serviceOri, instance string
	index := strings.LastIndex(service, "@")
	if index == -1 {
		serviceOri = service
	} else {
		serviceOri = service[0:index]
		instance = service[index+1:]
	}
	return &Route{server, service, method, serviceOri, instance}
}

// String transforms the route into a string
func (r *Route) String() string {
	if r.SvType != "" {
		return fmt.Sprintf("%s.%s.%s", r.SvType, r.Service, r.Method)
	}
	return r.Short()
}

// StringNotInst transforms the route into a string without the instance
func (r *Route) StringNotInst() string {
	if r.SvType != "" {
		return fmt.Sprintf("%s.%s.%s", r.SvType, r.ServiceOri, r.Method)
	}
	return r.Short()
}

// Short transforms the route into a string without the server type
func (r *Route) Short() string {
	return fmt.Sprintf("%s.%s", r.Service, r.Method)
}

// SetInstance sets the instance
func (r *Route) SetInstance(instance string) {
	r.Instance = instance
	if instance == "" {
		r.Service = r.ServiceOri
	} else {
		r.Service = fmt.Sprintf("%s@%s", r.ServiceOri, instance)
	}
}

// ServiceKey returns the service key
func (r *Route) ServiceKey() string {
	index := strings.LastIndex(r.Service, "@")
	if index == -1 {
		return fmt.Sprintf("%s.%s", r.Service, r.Method)
	}
	return fmt.Sprintf("%s.%s", r.Service[0:index], r.Method)
}

// Decode decodes the route
func Decode(route string) (*Route, error) {
	r := strings.Split(route, ".")
	for _, s := range r {
		if strings.TrimSpace(s) == "" {
			return nil, ErrRouteFieldCantEmpty
		}
	}
	switch len(r) {
	case 3:
		return NewRoute(r[0], r[1], r[2]), nil
	case 2:
		return NewRoute("", r[0], r[1]), nil
	default:
		logger.Log.Errorf("invalid route: " + route)
		return nil, ErrInvalidRoute
	}
}
