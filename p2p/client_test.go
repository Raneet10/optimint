package p2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lazyledger/optimint/config"
)

// TODO(tzdybal): move to some common place
type TestLogger struct {
	t *testing.T
}

func (t *TestLogger) Debug(msg string, keyvals ...interface{}) {
	t.t.Helper()
	t.t.Log(append([]interface{}{"DEBUG: " + msg}, keyvals...)...)
}

func (t *TestLogger) Info(msg string, keyvals ...interface{}) {
	t.t.Helper()
	t.t.Log(append([]interface{}{"INFO:  " + msg}, keyvals...)...)
}

func (t *TestLogger) Error(msg string, keyvals ...interface{}) {
	t.t.Helper()
	t.t.Log(append([]interface{}{"ERROR: " + msg}, keyvals...)...)
}

type MockLogger struct {
	debug, info, err []string
}

func (t *MockLogger) Debug(msg string, keyvals ...interface{}) {
	t.debug = append(t.debug, fmt.Sprint(append([]interface{}{msg}, keyvals...)...))
}

func (t *MockLogger) Info(msg string, keyvals ...interface{}) {
	t.info = append(t.info, fmt.Sprint(append([]interface{}{msg}, keyvals...)...))
}

func (t *MockLogger) Error(msg string, keyvals ...interface{}) {
	t.err = append(t.err, fmt.Sprint(append([]interface{}{msg}, keyvals...)...))
}

func TestClientStartup(t *testing.T) {
	privKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	client, err := NewClient(config.P2PConfig{}, privKey, "TestChain", &TestLogger{t})
	assert := assert.New(t)
	assert.NoError(err)
	assert.NotNil(client)

	err = client.Start(context.Background())
	defer client.Close()
	assert.NoError(err)
}

func TestBootstrapping(t *testing.T) {
	log.SetLogLevel("dht", "INFO")
	//log.SetDebugLogging()

	assert := assert.New(t)
	logger := &TestLogger{t}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clients := startTestNetwork(ctx, t, 4, map[int]hostDescr{
		1: hostDescr{conns: []int{0}},
		2: hostDescr{conns: []int{0, 1}},
		3: hostDescr{conns: []int{0}},
	}, logger)

	// wait for clients to finish refreshing routing tables
	clients.WaitForDHT()

	for _, client := range clients {
		assert.Equal(3, len(client.host.Network().Peers()))
	}
}

func TestDiscovery(t *testing.T) {
	assert := assert.New(t)
	logger := &TestLogger{t}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clients := startTestNetwork(ctx, t, 5, map[int]hostDescr{
		1: hostDescr{conns: []int{0}, chainID: "ORU2"},
		2: hostDescr{conns: []int{0}, chainID: "ORU2"},
		3: hostDescr{conns: []int{1}, chainID: "ORU1"},
		4: hostDescr{conns: []int{2}, chainID: "ORU1"},
	}, logger)

	// wait for clients to finish refreshing routing tables
	clients.WaitForDHT()

	assert.Contains(clients[3].host.Network().Peers(), clients[4].host.ID())
	assert.Contains(clients[4].host.Network().Peers(), clients[3].host.ID())
}

func TestGossiping(t *testing.T) {
	assert := assert.New(t)
	logger := &TestLogger{t}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clients := startTestNetwork(ctx, t, 5, map[int]hostDescr{
		1: hostDescr{conns: []int{0}, chainID: "test"},
		2: hostDescr{conns: []int{0}, chainID: "test"},
		3: hostDescr{conns: []int{1}, chainID: "test"},
		4: hostDescr{conns: []int{2}, chainID: "test", realKey: true},
	}, logger)

	// wait for clients to finish refreshing routing tables
	clients.WaitForDHT()

	time.Sleep(10*time.Second)

	// gossip from client 4
	err := clients[4].GossipTx(ctx, []byte("sample tx"))
	assert.NoError(err)

	nCtx, nCancel := context.WithTimeout(ctx, 300*time.Second)
	defer nCancel()
	msg, err := clients[2].txSub.Next(nCtx)
	assert.NoError(err)
	assert.NotNil(msg)
}

func TestSeedStringParsing(t *testing.T) {
	t.Parallel()

	privKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)

	seed1 := "/ip4/127.0.0.1/tcp/7676/p2p/12D3KooWM1NFkZozoatQi3JvFE57eBaX56mNgBA68Lk5MTPxBE4U"
	seed1MA, err := multiaddr.NewMultiaddr(seed1)
	require.NoError(t, err)
	seed1AI, err := peer.AddrInfoFromP2pAddr(seed1MA)
	require.NoError(t, err)

	seed2 := "/ip4/127.0.0.1/tcp/7677/p2p/12D3KooWAPRFbmWF5dAXvxLnEDxiHWhUuApVDpNNZwShiFAiJqrj"
	seed2MA, err := multiaddr.NewMultiaddr(seed2)
	require.NoError(t, err)
	seed2AI, err := peer.AddrInfoFromP2pAddr(seed2MA)
	require.NoError(t, err)

	// this one is a valid multiaddr, but can't be converted to PeerID (because there is no ID)
	seed3 := "/ip4/127.0.0.1/tcp/12345"

	cases := []struct {
		name     string
		input    string
		expected []peer.AddrInfo
		nErrors  int
	}{
		{"empty input", "", []peer.AddrInfo{}, 0},
		{"one correct seed", seed1, []peer.AddrInfo{*seed1AI}, 0},
		{"two correct seeds", seed1 + "," + seed2, []peer.AddrInfo{*seed1AI, *seed2AI}, 0},
		{"one wrong, two correct", "/ip4/," + seed1 + "," + seed2, []peer.AddrInfo{*seed1AI, *seed2AI}, 1},
		{"empty, two correct", "," + seed1 + "," + seed2, []peer.AddrInfo{*seed1AI, *seed2AI}, 1},
		{"empty, correct, empty, correct ", "," + seed1 + ",," + seed2, []peer.AddrInfo{*seed1AI, *seed2AI}, 2},
		{"invalid id, two correct", seed3 + "," + seed1 + "," + seed2, []peer.AddrInfo{*seed1AI, *seed2AI}, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)
			logger := &MockLogger{}
			client, err := NewClient(config.P2PConfig{}, privKey, "TestNetwork", logger)
			require.NoError(err)
			require.NotNil(client)
			actual := client.getSeedAddrInfo(c.input)
			assert.NotNil(actual)
			assert.Equal(c.expected, actual)
			// ensure that errors are logged
			assert.Equal(c.nErrors, len(logger.err))
		})
	}
}
