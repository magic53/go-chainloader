package btc

import (
	"bufio"
	"github.com/blocknetdx/go-exrplugins/data"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"io"
	"sync"
)

type Plugin struct {
	mu        sync.RWMutex
	cfg       *chaincfg.Params
	blocksDir string
	isReady   bool
	txIndex   map[wire.OutPoint]*data.BlockTx
	txCache   map[string]map[string]*data.Tx
	tokenCfg  *data.TokenConfig
}

// Mu returns the readwrite mutex.
func (bp *Plugin) Mu() *sync.RWMutex {
	return &bp.mu
}

// BlocksDir returns the location of all block dat files.
func (bp *Plugin) BlocksDir() string {
	return bp.blocksDir
}

// Ticker returns the ticker symbol (e.g. BLOCK, BTC, LTC).
func (bp *Plugin) Ticker() string {
	return data.PluginTicker(bp, "BTC")
}

// Ready returns true if the block db has loaded.
func (bp *Plugin) Ready() bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.isReady
}

// setReady state on the plugin.
func (bp *Plugin) SetReady() {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.isReady = true
}

// Network returns the network magic number.
func (bp *Plugin) Network() wire.BitcoinNet {
	return bp.cfg.Net
}

// Config returns the network magic number.
func (bp *Plugin) Config() chaincfg.Params {
	return *bp.cfg
}

// TokenConf returns the token configuration.
func (bp *Plugin) TokenConf() data.TokenConfig {
	return *bp.tokenCfg
}

// TxIndex returns the transaction index (lookup by outpoint).
func (bp *Plugin) TxIndex() map[wire.OutPoint]*data.BlockTx {
	return bp.txIndex
}

// TxCache returns the transaction cache
func (bp *Plugin) TxCache() map[string]map[string]*data.Tx {
	return bp.txCache
}

// SegwitActivated returns the segwit activation unix time.
func (bp *Plugin) SegwitActivated() int64 {
	return data.PluginSegwitActivated(bp, 1503539857)
}

// LoadBlocks load all transactions in the blocks dir.
func (bp *Plugin) LoadBlocks(blocksDir string) error {
	return data.PluginLoadBlocks(bp, blocksDir)
}

// ReadBlock deserializes bytes into block
func (bp *Plugin) ReadBlock(buf io.ReadSeeker) (*wire.MsgBlock, error) {
	return data.PluginReadBlock(bp, buf)
}

// ReadBlockHeader deserializes block header.
func (bp *Plugin) ReadBlockHeader(buf io.ReadSeeker) (header *wire.BlockHeader, err error) {
	header, err = data.PluginReadBlockHeader(bp, buf)
	return
}

// ReadTransaction deserializes bytes into transaction
func (bp *Plugin) ReadTransaction(buf io.ReadSeeker) (*wire.MsgTx, error) {
	return data.PluginReadTransaction(bp, buf)
}

// ReadBlocks loads block from the reader.
func (bp *Plugin) ReadBlocks(sc *bufio.Reader) ([]*data.ChainBlock, error) {
	return data.PluginReadBlocks(bp, sc, data.NetworkLE(bp.Network()))
}

// ProcessBlocks will process all blocks in the buffer.
func (bp *Plugin) ProcessBlocks(sc *bufio.Reader) ([]*data.Tx, error) {
	return data.PluginProcessBlocks(bp, sc)
}

// ProcessTxShard processes all transactions over specified range of blocks.
// Creates all send and receive transactions in the range. This func is
// thread safe.
func (bp *Plugin) ProcessTxShard(blocks []*data.ChainBlock, start, end int, wg *sync.WaitGroup) []*data.Tx {
	return data.PluginProcessTxShard(bp, blocks, start, end, wg)
}

// ProcessTxs processes and consolidates send and receive transactions.
func (bp *Plugin) ProcessTxs(sendTxs []*data.Tx, receiveTxs []*data.Tx) []*data.Tx {
	return data.PluginProcessTxs(bp, sendTxs, receiveTxs)
}

// AddAddrToCache adds the transaction to the cache.
func (bp *Plugin) AddTransactionToCache(tx *data.Tx) {
	data.PluginAddAddrToCache(bp, tx)
}

// AddBlocks process new blocks received by the network to keep internal chain data
// up to date.
func (bp *Plugin) AddBlocks(blocks []byte) ([]*data.Tx, error) {
	return data.PluginAddBlocks(bp, blocks)
}

// ImportTransactions imports the specified transactions into the data store.
func (bp *Plugin) ImportTransactions(transactions []*wire.MsgTx) ([]*data.Tx, error) {
	return data.PluginImportTransactions(bp, transactions)
}

// NewPlugin returns new BLOCK plugin instance.
func NewPlugin(cfg *chaincfg.Params, blocksDir string, tokenCfg *data.TokenConfig) *Plugin {
	plugin := &Plugin{
		cfg:       cfg,
		blocksDir: blocksDir,
		isReady:   false,
		txCache:   make(map[string]map[string]*data.Tx),
		txIndex:   make(map[wire.OutPoint]*data.BlockTx),
		tokenCfg:  tokenCfg,
	}
	return plugin
}
