package helpers

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	gnatsd "github.com/nats-io/nats-server/v2/test"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

// GetFreePort returns a free port
func GetFreePort(t testing.TB) int {
	t.Helper()
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// GetMapKeys returns a string slice with the map keys
func GetMapKeys(t *testing.T, m interface{}) []string {
	if reflect.ValueOf(m).Kind() != reflect.Map {
		t.Fatal(errors.New("GetMapKeys should receive a map"))
	}
	if reflect.TypeOf(m).Key() != reflect.TypeOf("bla") {
		t.Fatal(errors.New("GetMapKeys should receive a map with string keys"))
	}
	t.Helper()
	res := make([]string, 0)
	for _, k := range reflect.ValueOf(m).MapKeys() {
		res = append(res, k.String())
	}
	return res
}

// GetTestNatsServer gets a test nats server
func GetTestNatsServer(t *testing.T) *server.Server {
	opts := gnatsd.DefaultTestOptions
	port := GetFreePort(t)
	opts.Port = port
	s := gnatsd.RunServer(&opts)
	return s
}

// GetTestEtcd gets a test in memory etcd server
func GetTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()

	clientLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	clientAddr := clientLn.Addr().String()
	_ = clientLn.Close()

	peerLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	peerAddr := peerLn.Addr().String()
	_ = peerLn.Close()

	clientURL, _ := url.Parse("http://" + clientAddr)
	peerURL, _ := url.Parse("http://" + peerAddr)

	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()

	cfg.ListenClientUrls = []url.URL{*clientURL}
	cfg.AdvertiseClientUrls = []url.URL{*clientURL}

	cfg.ListenPeerUrls = []url.URL{*peerURL}
	cfg.AdvertisePeerUrls = []url.URL{*peerURL}

	cfg.InitialCluster = cfg.InitialClusterFromName(cfg.Name)

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		e.Server.Stop()
		t.Fatal("etcd server took too long to start")
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{clientURL.String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cli.Close()
		e.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = cli.Get(ctx, "health")
	if err != nil {
		t.Fatal(err)
	}

	return e, cli
}

// WriteFile test helper
func WriteFile(t *testing.T, filepath string, bytes []byte) {
	t.Helper()
	if err := os.WriteFile(filepath, bytes, 0644); err != nil {
		t.Fatalf("failed writing file: %s", err)
	}
}

// ReadFile test helper
func ReadFile(t *testing.T, filepath string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath)
	if err != nil {
		t.Fatalf("failed reading file: %s", err)
	}
	return b
}

// StartProcess starts a process
func StartProcess(t testing.TB, program string, args ...string) *exec.Cmd {
	t.Helper()
	return exec.Command(program, args...)
}

func waitForServerToBeReady(t testing.TB, out *bufio.Reader) {
	t.Helper()
	ShouldEventuallyReturn(t, func() bool {
		line, _, err := out.ReadLine()
		if err != nil {
			t.Fatal(err)
		}
		return strings.Contains(string(line), "all modules started!")
	}, true, 100*time.Millisecond, 30*time.Second)
}

func StartServer(
	t testing.TB,
	frontend, debug bool,
	svType string,
	port int,
	sdPrefix string,
	grpc, lazyConnection bool,
	envVars ...string,
) func() {
	return startServer(t, frontend, debug, svType, port, sdPrefix, grpc, lazyConnection, false, envVars...)
}

func StartServerWithLoopback(
	t testing.TB,
	frontend, debug bool,
	svType string,
	port int,
	sdPrefix string,
	grpc, lazyConnection bool,
) func() {
	return startServer(t, frontend, debug, svType, port, sdPrefix, grpc, lazyConnection, true)
}

// StartServer starts a server
func startServer(
	t testing.TB,
	frontend, debug bool,
	svType string,
	port int,
	sdPrefix string,
	grpc, lazyConnection, loopback bool,
	envVars ...string,
) func() {
	grpcPort := GetFreePort(t)
	promPort := GetFreePort(t)
	var useGRPC string
	if grpc {
		useGRPC = "true"
	} else {
		useGRPC = "false"
	}
	t.Helper()
	args := []string{
		"-type",
		svType,
		"-port",
		strconv.Itoa(port),
		fmt.Sprintf("-frontend=%s", strconv.FormatBool(frontend)),
		"-sdprefix", sdPrefix,
		"-grpcport", fmt.Sprintf("%d", grpcPort),
		fmt.Sprintf("-grpc=%s", useGRPC),
	}
	if debug {
		args = append(args, "-debug")
	}
	cmd := StartProcess(
		t,
		"../examples/testing/server",
		args...,
	)

	// always use a random port for prometheus, to avoid e2e conflicts
	cmd.Env = append(envVars, []string{
		fmt.Sprintf("PITAYA_METRICS_PROMETHEUS_PORT=%d", promPort),
		fmt.Sprintf("PITAYA_CLUSTER_RPC_CLIENT_GRPC_LAZYCONNECTION=%v", lazyConnection),
		fmt.Sprintf("PITAYA_CLUSTER_RPC_SERVER_LOOPBACKENABLED=%v", loopback),
	}...)

	outPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	waitForServerToBeReady(t, bufio.NewReader(outPipe))
	return func() {
		if err := cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}
	}
}

// FixtureGoldenFileName returns the golden file name on fixtures path
func FixtureGoldenFileName(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("fixtures", name+".golden")
}

func vetExtras(extras []interface{}) (bool, string) {
	for i, extra := range extras {
		if extra != nil {
			zeroValue := reflect.Zero(reflect.TypeOf(extra)).Interface()
			if !reflect.DeepEqual(zeroValue, extra) {
				message := fmt.Sprintf("unexpected non-nil/non-zero extra argument at index %d:\n\t<%T>: %#v", i+1, extra, extra)
				return false, message
			}
		}
	}
	return true, ""
}

func pollFuncReturn(f interface{}) (interface{}, error) {
	values := reflect.ValueOf(f).Call([]reflect.Value{})

	extras := []interface{}{}
	for _, value := range values[1:] {
		extras = append(extras, value.Interface())
	}

	success, message := vetExtras(extras)

	if !success {
		return nil, errors.New(message)
	}

	return values[0].Interface(), nil
}

// ShouldEventuallyReceive should asserts that eventually channel c receives a value
func ShouldEventuallyReceive(t testing.TB, c interface{}, timeouts ...time.Duration) interface{} {
	t.Helper()
	if !isChan(c) {
		t.Fatal("ShouldEventuallyReceive c argument should be a channel")
	}
	v := reflect.ValueOf(c)

	timeout := time.After(500 * time.Millisecond)

	if len(timeouts) > 0 {
		timeout = time.After(timeouts[0])
	}

	recvChan := make(chan reflect.Value)

	go func() {
		v, ok := v.Recv()
		if ok {
			recvChan <- v
		}
	}()

	select {
	case <-timeout:
		t.Fatal(errors.New("timed out waiting for channel to receive"))
	case a := <-recvChan:
		return a.Interface()
	}

	return nil
}

// ShouldAlwaysReturn asserts that the return of f should always be v, timeouts: 0 - evaluation interval, 1 - timeout
func ShouldAlwaysReturn(t testing.TB, f interface{}, v interface{}, timeouts ...time.Duration) {
	t.Helper()
	interval := 10 * time.Millisecond
	timeout := time.After(50 * time.Millisecond)
	switch len(timeouts) {
	case 1:
		interval = timeouts[0]
		break
	case 2:
		interval = timeouts[0]
		timeout = time.After(timeouts[1])
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if isFunction(f) {
		for {
			select {
			case <-timeout:
				return
			case <-ticker.C:
				val, err := pollFuncReturn(f)
				if err != nil {
					t.Fatal(err)
				}
				if v != val {
					t.Fatalf("function f returned wrong value %s", val)
				}
			}
		}
	} else {
		t.Fatal("ShouldAlwaysReturn should receive a function with no args and more than 0 outs")
		return
	}
}

// ShouldEventuallyReturn asserts that eventually the return of f should be v, timeouts: 0 - evaluation interval, 1 - timeout
func ShouldEventuallyReturn(t testing.TB, f interface{}, v interface{}, timeouts ...time.Duration) {
	t.Helper()
	interval := 10 * time.Millisecond
	timeout := time.After(500 * time.Millisecond)
	switch len(timeouts) {
	case 1:
		interval = timeouts[0]
		break
	case 2:
		interval = timeouts[0]
		timeout = time.After(timeouts[1])
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if isFunction(f) {
		for {
			select {
			case <-timeout:
				t.Fatalf("function f never returned value %s", v)
			case <-ticker.C:
				val, err := pollFuncReturn(f)
				if err != nil {
					t.Fatal(err)
				}
				if v == val {
					return
				}
			}
		}
	} else {
		t.Fatal("ShouldEventuallyEqual should receive a function with no args and more than 0 outs")
		return
	}
}
