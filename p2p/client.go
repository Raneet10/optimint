package p2p

import (
	"context"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/log"
	"github.com/multiformats/go-multiaddr"
)

// Client is a P2P client, implemented with libp2p
type Client struct {
	conf    config.P2PConfig
	privKey crypto.PrivKey

	host host.Host

	logger log.Logger
}

// NewClient creates new Client object
//
// Basic checks on parameters are done, and default parameters are provided for unset-configuration
func NewClient(conf config.P2PConfig, privKey crypto.PrivKey, logger log.Logger) (*Client, error) {
	if privKey == nil {
		return nil, ErrNoPrivKey
	}
	if conf.ListenAddress == "" {
		// TODO(tzdybal): extract const
		conf.ListenAddress = "/ip4/127.0.0.1/tcp/7676"
	}
	return &Client{
		conf:    conf,
		privKey: privKey,
		logger:  logger,
	}, nil
}

func (c *Client) Start() error {
	c.logger.Debug("Starting P2P client")
	err := c.listen()
	if err != nil {
		return err
	}

	// start bootstrapping connections
	err = c.bootstrap()
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) listen() error {
	// TODO(tzdybal): consider requiring listen address in multiaddress format
	maddr, err := multiaddr.NewMultiaddr(c.conf.ListenAddress)
	if err != nil {
		return err
	}

	//TODO(tzdybal): think about per-client context
	host, err := libp2p.New(context.Background(), libp2p.ListenAddrs(maddr), libp2p.Identity(c.privKey))
	if err != nil {
		return err
	}
	for _, a := range host.Addrs() {
		c.logger.Info("listening on", "address", a, "ID", host.ID())
	}

	c.host = host
	return nil
}

func (c *Client) bootstrap() error {
	if len(strings.TrimSpace(c.conf.Seeds)) == 0 {
		c.logger.Info("no seed nodes - only listening for connections")
		return nil
	}
	seeds := strings.Split(c.conf.Seeds, ",")
	for _, s := range seeds {
		maddr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			c.logger.Error("error while parsing seed node", "address", s, "error", err)
			continue
		}
		c.logger.Debug("seed", "addr", maddr.String())
		// TODO(tzdybal): configuration param for connection timeout
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		addrInfo, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			c.logger.Error("error while creating address info", "error", err)
			continue
		}
		err = c.host.Connect(ctx, *addrInfo)
		if err != nil {
			c.logger.Error("error while connecting to seed node", "error", err)
			continue
		}
		c.logger.Debug("connected to seed node", "address", s)
	}

	return nil
}
