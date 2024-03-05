package celestia_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	celestia "github.com/ethereum-optimism/optimism/op-celestia"
	plasma "github.com/ethereum-optimism/optimism/op-plasma"
	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"google.golang.org/grpc/credentials/insecure"

	"github.com/rollkit/go-da/proxy"
	goDATest "github.com/rollkit/go-da/test"
)

func TestMain(m *testing.M) {
	srv := startMockGRPCServ()
	if srv == nil {
		os.Exit(1)
	}
	exitCode := m.Run()

	// teardown servers
	srv.GracefulStop()

	os.Exit(exitCode)
}

func TestCelestia(t *testing.T) {
	dummyClient := &celestia.DAClient{DA: goDATest.NewDummyDA(), GetTimeout: time.Minute, Log: log.New()}
	grpcClient, err := startMockGRPCClient()
	require.NoError(t, err)
	clients := map[string]*celestia.DAClient{
		"dummy": dummyClient,
		"grpc":  grpcClient,
	}
	tests := []struct {
		name string
		f    func(t *testing.T, da plasma.DAStorage)
	}{
		{"roundtrip", doTestRoundTrip},
		{"missing", doTestMissing},
	}
	for name, dalc := range clients {
		for _, tc := range tests {
			t.Run(name+"_"+tc.name, func(t *testing.T) {
				tc.f(t, dalc)
			})
		}
	}
}

func getRandomBytes(size int) []byte {
	data := make([]byte, size)
	_, _ = rand.Read(data) //nolint:gosec,staticcheck
	return data
}

func doTestRoundTrip(t *testing.T, da plasma.DAStorage) {
	img := getRandomBytes(256)
	key, err := da.SetInput(context.TODO(), img)
	require.NoError(t, err)
	got, err := da.GetInput(context.TODO(), key)
	require.NoError(t, err)
	require.Equal(t, img, got)
}

func doTestMissing(t *testing.T, da plasma.DAStorage) {
	key := getRandomBytes(32)
	_, err := da.GetInput(context.TODO(), key)
	require.Error(t, err)
}

func startMockGRPCServ() *grpc.Server {
	srv := proxy.NewServer(goDATest.NewDummyDA(), grpc.Creds(insecure.NewCredentials()))
	lis, err := net.Listen("tcp", "127.0.0.1"+":"+strconv.Itoa(7980))
	if err != nil {
		fmt.Println(err)
		return nil
	}
	go func() {
		_ = srv.Serve(lis)
	}()
	return srv
}

func startMockGRPCClient() (*celestia.DAClient, error) {
	client := proxy.NewClient()
	err := client.Start("127.0.0.1:7980", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &celestia.DAClient{DA: client, GetTimeout: time.Minute, Log: log.New()}, nil
}
