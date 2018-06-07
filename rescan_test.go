package neutrino_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcwallet/walletdb"
	_ "github.com/btcsuite/btcwallet/walletdb/bdb"
	"github.com/lightninglabs/neutrino"
)

var (
	netParams = &chaincfg.RegressionNetParams
)

func TestRescan(t *testing.T) {
	// Initialize the harness around a btcd node which will serve as our
	// dedicated miner to generate blocks
	miner, err := rpctest.New(netParams, nil, nil)
	if err != nil {
		t.Fatalf("unable to create mining node: %v", err)
	}
	defer miner.TearDown()
	if err := miner.SetUp(true, 25); err != nil {
		t.Fatalf("unable to set up mining node: %v", err)
	}

	// create neutrino database and temporary folder for the data
	spvDir, err := ioutil.TempDir("", "neutrino")
	if err != nil {
		t.Fatalf("unable to create temp dir: %v", err)
	}
	fmt.Printf("SPV dir is on: %s \n", spvDir)

	dbName := filepath.Join(spvDir, "neutrino.db")
	spvDatabase, err := walletdb.Create("bdb", dbName)
	if err != nil {
		t.Fatalf("unable to create walletdb: %v", err)
	}

	// init spv client connected to miner
	//rpcConfig := miner.RPCConfig()
	p2pAddr := miner.P2PAddress()

	chainService, err := neutrino.NewChainService(neutrino.Config{
		DataDir:      spvDir,
		Database:     spvDatabase,
		ChainParams:  *netParams,
		ConnectPeers: []string{p2pAddr},
	})
	if err != nil {
		t.Fatal(err)
	}

	chainService.Start()

	cleanUp := func() {
		chainService.Stop()
		spvDatabase.Close()
		os.RemoveAll(spvDir)
	}

	// Start rescanning process with virtual node
	startTime := time.Now()
	rescanQuit := make(chan struct{})

	onBlockConnected := func(hash *chainhash.Hash, height int32, t time.Time) {
		fmt.Printf("onBlockConnected: height(%v), hash(%v), time: %v", height, hash, t)
	}
	onFilteredBlockConnected := func(height int32, header *wire.BlockHeader, txs []*btcutil.Tx) {
		fmt.Printf("onFilteredBlockConnected: height(%v), hash(%v)", height, header.BlockHash())
	}
	onBlockDisconnected := func(hash *chainhash.Hash, height int32, t time.Time) {
		fmt.Printf("onBlockDisconnected: height(%v), hash(%v), time: %v", height, hash, t)
	}


	miner.Node.Generate(350)

	_, currentHeight, err := miner.Node.GetBestBlock()
	if err != nil {
		t.Fatalf("unable to get current height: %v", err)
	}
	fmt.Println("currentHeight", currentHeight)

	newRescan := chainService.NewRescan(
		neutrino.NotificationHandlers(rpcclient.NotificationHandlers{
			OnBlockConnected:         onBlockConnected,
			OnFilteredBlockConnected: onFilteredBlockConnected,
			OnBlockDisconnected:      onBlockDisconnected,
		}),
		neutrino.StartTime(startTime),
		neutrino.QuitChan(rescanQuit),
		neutrino.WatchAddrs([]btcutil.Address{}...),
	)
	newRescan.Start()

	miner.Node.Generate(1)
	time.Sleep(time.Second)
	miner.Node.Generate(1)

	time.Sleep(time.Second * 15)

	cleanUp()
}

