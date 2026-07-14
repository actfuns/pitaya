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

package pitaya

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/actfuns/pitaya/v2/component"
	"github.com/actfuns/pitaya/v2/config"
	"github.com/actfuns/pitaya/v2/prpc"
)

type MyComp struct {
	component.Base
	running bool
}

func (m *MyComp) Init() {
	m.running = true
}

func (m *MyComp) Shutdown() {
	m.running = false
}

func TestRegister(t *testing.T) {
	config := config.NewDefaultPitayaConfig()
	app := NewDefaultApp(true, "testtype", Cluster, map[string]string{}, *config).(*App)
	before := len(app.handlerComp)
	b := &component.Base{}
	app.Register(&prpc.ServiceDesc{DomainName: "test", ServiceName: "test"}, b)
	assert.Equal(t, before+1, len(app.handlerComp))
}

func TestStartupComponents(t *testing.T) {
	app := NewDefaultApp(true, "testtype", Standalone, map[string]string{}, *config.NewDefaultPitayaConfig()).(*App)

	app.Register(&prpc.ServiceDesc{DomainName: "test", ServiceName: "test"}, &MyComp{})
	app.startupComponents()
	idx := len(app.handlerComp) - 1
	assert.Equal(t, true, app.handlerComp[idx].comp.(*MyComp).running)
}

func TestShutdownComponents(t *testing.T) {
	app := NewDefaultApp(true, "testtype", Standalone, map[string]string{}, *config.NewDefaultPitayaConfig()).(*App)

	app.Register(&prpc.ServiceDesc{DomainName: "test", ServiceName: "test"}, &MyComp{})
	app.startupComponents()

	app.shutdownComponents()
	idx := len(app.handlerComp) - 1
	assert.Equal(t, false, app.handlerComp[idx].comp.(*MyComp).running)
}
