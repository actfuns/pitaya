package router

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/actfuns/pitaya/v2/cluster"
	"github.com/actfuns/pitaya/v2/cluster/mocks"
	"github.com/actfuns/pitaya/v2/conn/message"
	"github.com/actfuns/pitaya/v2/constants"
	"github.com/actfuns/pitaya/v2/protos"
	"github.com/actfuns/pitaya/v2/route"
	"go.uber.org/mock/gomock"
)

var (
	serverID        = "id"
	serverType      = "serverType"
	frontend        = true
	server          = cluster.NewServer(serverID, serverType, frontend, cluster.WithDomain(serverType))
	routingFunction = func(
		ctx context.Context,
		rpcType protos.RPCType,
		route *route.Route,
		payload []byte,
	) (context.Context, string, *cluster.Server, error) {
		return ctx, route.Domain, server, nil
	}
)

var routerTables = map[string]struct {
	server  *cluster.Server
	rpcType protos.RPCType
	err     error
}{
	"test_server_has_route_func":   {server, protos.RPCType_Sys, nil},
	"test_server_use_default_func": {server, protos.RPCType_Sys, nil},
	"test_user_use_default_func":   {server, protos.RPCType_User, nil},
	"test_error_on_service_disc":   {server, protos.RPCType_Sys, nil},
}

var addRouteRouterTables = map[string]struct {
	serverType string
}{
	"test_overrige_server_type": {serverType},
	"test_new_server_type":      {"notRegisteredType"},
}

func TestNew(t *testing.T) {
	t.Parallel()
	router := New()
	assert.NotNil(t, router)
}

func TestDefaultRoute(t *testing.T) {
	router := New()
	router.SetServer(server)
	route := route.NewRoute(serverType, "service", "method")

	_, _, retServer, _ := router.defaultRoute(context.Background(), route)
	assert.Equal(t, server, retServer)
}

func TestRoute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	route := route.NewRoute(serverType, "service", "method")

	for name, table := range routerTables {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockServiceDiscovery := mocks.NewMockServiceDiscovery(ctrl)

			router := New()
			router.AddRoute(serverType, routingFunction)
			router.SetServiceDiscovery(mockServiceDiscovery)

			_, _, retServer, err := router.Resolve(ctx, table.rpcType, route, &message.Message{
				Data: []byte{0x01},
			})
			assert.Equal(t, table.server, retServer)
			assert.Equal(t, table.err, err)
		})
	}
}

func TestAddRoute(t *testing.T) {
	t.Parallel()

	for name, table := range addRouteRouterTables {
		t.Run(name, func(t *testing.T) {
			router := New()
			router.AddRoute(table.serverType, routingFunction)

			assert.NotNil(t, router.routesMap[table.serverType])
			assert.Nil(t, router.routesMap["anotherServerType"])
		})
	}
}

func TestRouteFailIfNullServiceDiscovery(t *testing.T) {
	t.Parallel()

	router := New()
	_, _, _, err := router.Resolve(context.Background(), protos.RPCType_Sys, route.NewRoute(serverType, "service", "method"), &message.Message{
		Data: []byte{0x01},
	})
	assert.Equal(t, constants.ErrServiceDiscoveryNotInitialized, err)
}
