// Copyright 2019 The go-relianz Authors
// This file is part of the go-relianz library.
//
// The go-relianz library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-relianz library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-relianz library. If not, see <http://www.gnu.org/licenses/>.

// Package les implements the Light Relianz Subprotocol.
package les

import (
	"fmt"
	"sync"
	"time"

	"github.com/relianz2019/relianz/accounts"
	"github.com/relianz2019/relianz/common"
	"github.com/relianz2019/relianz/common/hexutil"
	"github.com/relianz2019/relianz/consensus"
	"github.com/relianz2019/relianz/core"
	"github.com/relianz2019/relianz/core/bloombits"
	"github.com/relianz2019/relianz/core/rawdb"
	"github.com/relianz2019/relianz/core/types"
	"github.com/relianz2019/relianz/rlz"
	"github.com/relianz2019/relianz/rlz/downloader"
	"github.com/relianz2019/relianz/rlz/filters"
	"github.com/relianz2019/relianz/rlz/gasprice"
	"github.com/relianz2019/relianz/ethdb"
	"github.com/relianz2019/relianz/event"
	"github.com/relianz2019/relianz/internal/ethapi"
	"github.com/relianz2019/relianz/light"
	"github.com/relianz2019/relianz/log"
	"github.com/relianz2019/relianz/node"
	"github.com/relianz2019/relianz/p2p"
	"github.com/relianz2019/relianz/p2p/discv5"
	"github.com/relianz2019/relianz/params"
	rpc "github.com/relianz2019/relianz/rpc"
)

type LightRelianz struct {
	config *rlz.Config

	odr         *LesOdr
	relay       *LesTxRelay
	chainConfig *params.ChainConfig
	// Channel for shutting down the service
	shutdownChan chan bool
	// Handlers
	peers           *peerSet
	txPool          *light.TxPool
	blockchain      *light.LightChain
	protocolManager *ProtocolManager
	serverPool      *serverPool
	reqDist         *requestDistributor
	retriever       *retrieveManager
	// DB interfaces
	chainDb ethdb.Database // Block chain database

	bloomRequests                              chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer, chtIndexer, bloomTrieIndexer *core.ChainIndexer

	ApiBackend *LesApiBackend

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	networkId     uint64
	netRPCService *ethapi.PublicNetAPI

	wg sync.WaitGroup
}

func New(ctx *node.ServiceContext, config *rlz.Config) (*LightRelianz, error) {
	chainDb, err := rlz.CreateDB(ctx, config, "lightchaindata")
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlock(chainDb, config.Genesis)
	if _, isCompat := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !isCompat {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)
	if chainConfig.Alien != nil {
		log.Info("Initialised alien configuration", "config", *chainConfig.Alien)
		if config.NetworkId == 1 {// rlz.DefaultConfig.NetworkId
			// change default rlz networkid  to default ttc networkid
			config.NetworkId = chainConfig.ChainId.Uint64()
		}
	}

	peers := newPeerSet()
	quitSync := make(chan struct{})

	leth := &LightRelianz{
		config:           config,
		chainConfig:      chainConfig,
		chainDb:          chainDb,
		eventMux:         ctx.EventMux,
		peers:            peers,
		reqDist:          newRequestDistributor(peers, quitSync),
		accountManager:   ctx.AccountManager,
		engine:           rlz.CreateConsensusEngine(ctx, &config.Rlzash, chainConfig, chainDb),
		shutdownChan:     make(chan bool),
		networkId:        config.NetworkId,
		bloomRequests:    make(chan chan *bloombits.Retrieval),
		bloomIndexer:     rlz.NewBloomIndexer(chainDb, light.BloomTrieFrequency),
		chtIndexer:       light.NewChtIndexer(chainDb, true),
		bloomTrieIndexer: light.NewBloomTrieIndexer(chainDb, true),
	}

	leth.relay = NewLesTxRelay(peers, leth.reqDist)
	leth.serverPool = newServerPool(chainDb, quitSync, &leth.wg)
	leth.retriever = newRetrieveManager(peers, leth.reqDist, leth.serverPool)
	leth.odr = NewLesOdr(chainDb, leth.chtIndexer, leth.bloomTrieIndexer, leth.bloomIndexer, leth.retriever)
	if leth.blockchain, err = light.NewLightChain(leth.odr, leth.chainConfig, leth.engine); err != nil {
		return nil, err
	}
	leth.bloomIndexer.Start(leth.blockchain)
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		leth.blockchain.SetHead(compat.RewindTo)
		rawdb.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}

	leth.txPool = light.NewTxPool(leth.chainConfig, leth.blockchain, leth.relay)
	if leth.protocolManager, err = NewProtocolManager(leth.chainConfig, true, ClientProtocolVersions, config.NetworkId, leth.eventMux, leth.engine, leth.peers, leth.blockchain, nil, chainDb, leth.odr, leth.relay, quitSync, &leth.wg); err != nil {
		return nil, err
	}
	leth.ApiBackend = &LesApiBackend{leth, nil}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.GasPrice
	}
	leth.ApiBackend.gpo = gasprice.NewOracle(leth.ApiBackend, gpoParams)
	return leth, nil
}

func lesTopic(genesisHash common.Hash, protocolVersion uint) discv5.Topic {
	var name string
	switch protocolVersion {
	case lpv1:
		name = "LES"
	case lpv2:
		name = "LES2"
	default:
		panic(nil)
	}
	return discv5.Topic(name + "@" + common.Bytes2Hex(genesisHash.Bytes()[0:8]))
}

type LightDummyAPI struct{}

// Rlzerbase is the address that mining rewards will be send to
func (s *LightDummyAPI) Rlzerbase() (common.Address, error) {
	return common.Address{}, fmt.Errorf("not supported")
}

// Coinbase is the address that mining rewards will be send to (alias for Rlzerbase)
func (s *LightDummyAPI) Coinbase() (common.Address, error) {
	return common.Address{}, fmt.Errorf("not supported")
}

// Hashrate returns the POW hashrate
func (s *LightDummyAPI) Hashrate() hexutil.Uint {
	return 0
}

// Mining returns an indication if this node is currently mining.
func (s *LightDummyAPI) Mining() bool {
	return false
}

// APIs returns the collection of RPC services the relianz package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *LightRelianz) APIs() []rpc.API {
	return append(ethapi.GetAPIs(s.ApiBackend), []rpc.API{
		{
			Namespace: "rlz",
			Version:   "1.0",
			Service:   &LightDummyAPI{},
			Public:    true,
		}, {
			Namespace: "rlz",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.protocolManager.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "rlz",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.ApiBackend, true),
			Public:    true,
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
}

func (s *LightRelianz) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *LightRelianz) BlockChain() *light.LightChain      { return s.blockchain }
func (s *LightRelianz) TxPool() *light.TxPool              { return s.txPool }
func (s *LightRelianz) Engine() consensus.Engine           { return s.engine }
func (s *LightRelianz) LesVersion() int                    { return int(s.protocolManager.SubProtocols[0].Version) }
func (s *LightRelianz) Downloader() *downloader.Downloader { return s.protocolManager.downloader }
func (s *LightRelianz) EventMux() *event.TypeMux           { return s.eventMux }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (s *LightRelianz) Protocols() []p2p.Protocol {
	return s.protocolManager.SubProtocols
}

// Start implements node.Service, starting all internal goroutines needed by the
// Relianz protocol implementation.
func (s *LightRelianz) Start(srvr *p2p.Server) error {
	s.startBloomHandlers()
	log.Warn("Light client mode is an experimental feature")
	s.netRPCService = ethapi.NewPublicNetAPI(srvr, s.networkId)
	// clients are searching for the first advertised protocol in the list
	protocolVersion := AdvertiseProtocolVersions[0]
	s.serverPool.start(srvr, lesTopic(s.blockchain.Genesis().Hash(), protocolVersion))
	s.protocolManager.Start(s.config.LightPeers)
	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Relianz protocol.
func (s *LightRelianz) Stop() error {
	s.odr.Stop()
	if s.bloomIndexer != nil {
		s.bloomIndexer.Close()
	}
	if s.chtIndexer != nil {
		s.chtIndexer.Close()
	}
	if s.bloomTrieIndexer != nil {
		s.bloomTrieIndexer.Close()
	}
	s.blockchain.Stop()
	s.protocolManager.Stop()
	s.txPool.Stop()

	s.eventMux.Stop()

	time.Sleep(time.Millisecond * 200)
	s.chainDb.Close()
	close(s.shutdownChan)

	return nil
}
