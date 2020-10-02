package data

import (
	"bufio"
	"bytes"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"sync"
	"time"
)

type ChainPlugin struct {
	mu          sync.RWMutex
	cfg         *chaincfg.Params
	blocksDir   string
	isReady     bool
	txIndex     map[wire.OutPoint]*BlockTx
	txCache     map[string]map[string]*Tx
	TokenCfg    *Token
	BlockReader BlockReader
}

type chainPluginBlockReader struct {
	tokenCfg *Token
}

// SegwitActivated returns the segwit activation unix time.
func (bp *chainPluginBlockReader) SegwitActivated() int64 {
	if bp.tokenCfg != nil {
		return bp.tokenCfg.SegwitActivated
	} else {
		return 0 // unix time
	}
}

// ReadBlock deserializes bytes into block
func (bp *chainPluginBlockReader) ReadBlock(buf io.ReadSeeker) (block *wire.MsgBlock, err error) {
	var header *wire.BlockHeader
	if header, err = bp.ReadBlockHeader(buf); err != nil {
		log.Println("failed to read block header", err.Error())
		return
	}
	block, err = ReadBlock(buf, header, bp.SegwitActivated())
	return
}

// ReadBlockHeader deserializes block header.
func (bp *chainPluginBlockReader) ReadBlockHeader(buf io.ReadSeeker) (header *wire.BlockHeader, err error) {
	header, err = ReadBlockHeader(buf)
	return
}

// ReadTransaction deserializes bytes into transaction
func (bp *chainPluginBlockReader) ReadTransaction(buf io.ReadSeeker) (tx *wire.MsgTx, err error) {
	tx, err = ReadTransaction(buf, time.Now().Unix() >= bp.SegwitActivated())
	return
}

// SegwitActivated returns the segwit activation unix time.
func (bp *ChainPlugin) SegwitActivated() int64 {
	return bp.BlockReader.SegwitActivated()
}

// BlocksDir returns the location of all block dat files.
func (bp *ChainPlugin) BlocksDir() string {
	return bp.blocksDir
}

// Ready returns true if the block db has loaded.
func (bp *ChainPlugin) Ready() bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.isReady
}

// Network returns the network magic number.
func (bp *ChainPlugin) Network() wire.BitcoinNet {
	return bp.cfg.Net
}

// Config returns the network magic number.
func (bp *ChainPlugin) Config() chaincfg.Params {
	return *bp.cfg
}

// ClearIndex removes references to the transaction index. This typically frees up
// significant memory.
func (bp *ChainPlugin) ClearIndex() {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.txIndex = map[wire.OutPoint]*BlockTx{}
}

// LoadBlocks load all transactions in the blocks dir.
func (bp *ChainPlugin) LoadBlocks(blocksDir string) (err error) {
	var files []os.FileInfo
	files, err = BlockFiles(blocksDir, regexp.MustCompile(`blk(\d+).dat`), 3)

	// Iterate oldest blocks first (filterFiles sorted descending)
	for _, file := range files {
		if IsShuttingDown() {
			return
		}
		path := filepath.Join(blocksDir, file.Name())
		var fs *os.File
		fs, err = os.Open(path)
		if err != nil {
			return
		}
		if _, err = bp.processBlocks(bufio.NewReader(fs)); err != nil {
			log.Printf("Error loading block database %s\n", path)
			_ = fs.Close()
			return
		} else {
			log.Printf("%s db file loaded: %s\n", bp.TokenCfg.Ticker, path)
			_ = fs.Close()
		}
	}
	log.Printf("Done loading %s database files\n", bp.TokenCfg.Ticker)

	bp.setReady()
	return
}

// ReadBlock deserializes bytes into block 
func (bp *ChainPlugin) ReadBlock(buf io.ReadSeeker) (block *wire.MsgBlock, err error) {
	block, err = bp.BlockReader.ReadBlock(buf)
	return
}

// ReadBlockHeader deserializes block header.
func (bp *ChainPlugin) ReadBlockHeader(buf io.ReadSeeker) (header *wire.BlockHeader, err error) {
	header, err = bp.BlockReader.ReadBlockHeader(buf)
	return
}

// ReadTransaction deserializes bytes into transaction 
func (bp *ChainPlugin) ReadTransaction(buf io.ReadSeeker) (tx *wire.MsgTx, err error) {
	tx, err = bp.BlockReader.ReadTransaction(buf)
	return
}

// AddBlocks process new blocks received by the network to keep internal chain data
// up to date.
func (bp *ChainPlugin) AddBlocks(blocks []byte) (txs []*Tx, err error) {
	r := bytes.NewReader(blocks)
	txs, err = bp.processBlocks(bufio.NewReader(r))
	return
}

// ImportTransactions imports the specified transactions into the data store.
func (bp *ChainPlugin) ImportTransactions(transactions []*wire.MsgTx) (txs []*Tx, err error) {
	bp.mu.RLock()
	sendTxs, receiveTxs := ProcessTransactions(bp, time.Now(), transactions, bp.txIndex)
	bp.mu.RUnlock()
	txs = bp.processTxs(sendTxs, receiveTxs)
	return
}

// setReady state on the plugin.
func (bp *ChainPlugin) setReady() {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.isReady = true
}

// loadBlocks loads block from the reader.
func (bp *ChainPlugin) loadBlocks(sc *bufio.Reader, network []byte) (blocks []*ChainBlock, err error) {
	_, err = NextBlock(sc, network, func(blockBytes []byte) bool {
		var wireBlock *wire.MsgBlock
		var err2 error
		if wireBlock, err2 = bp.ReadBlock(bytes.NewReader(blockBytes)); err2 != nil {
			log.Println("failed to read block", err2.Error())
			return true
		}
		block := newChainBlock(wireBlock)
		blocks = append(blocks, block)
		return true
	})

	if err == io.EOF { // not fatal
		err = nil
	}

	return
}

// processBlocks will process all blocks in the buffer.
func (bp *ChainPlugin) processBlocks(sc *bufio.Reader) (txs []*Tx, err error) {
	var blocks []*ChainBlock
	if blocks, err = bp.loadBlocks(sc, NetworkLE(bp.Network())); err != nil {
		log.Println("Error loading block database")
		return
	}

	// Produce block transaction index
	var txBlocks []*BlockTx
	blocksLen := len(blocks)
	shards, rng, remainder := ShardsData(runtime.NumCPU()*4, blocksLen)
	var wg sync.WaitGroup
	wg.Add(shards)
	mu := sync.Mutex{}
	for i := 0; i < shards; i++ {
		start, end := ShardsIter(shards, i, rng, remainder)
		go func() {
			var transactions []*BlockTx
			for j := start; j < end; j++ {
				wireBlock := blocks[j].Block()
				for _, tx := range wireBlock.Transactions {
					txHash := tx.TxHash()
					for n := range tx.TxOut {
						outp := wire.NewOutPoint(&txHash, uint32(n))
						transactions = append(transactions, &BlockTx{
							OutP:        outp,
							Transaction: tx,
						})
					}
				}
				if j%1000 == 0 && IsShuttingDown() {
					break
				}
			}
			mu.Lock()
			txBlocks = append(txBlocks, transactions...)
			mu.Unlock()
			wg.Done()
		}()
	}
	wg.Wait()

	// Update the tx index
	bp.mu.Lock()
	for _, txi := range txBlocks {
		bp.txIndex[*txi.OutP] = txi
	}
	bp.mu.Unlock()

	// Sort blocks by time ascending
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Block().Header.Timestamp.Unix() < blocks[j].Block().Header.Timestamp.Unix()
	})

	// Process transactions, spawn goroutines to process all current blocks
	// Use a multiplier of num cpu as a starting point, let go scheduler fill
	// in work.
	blocksLen = len(blocks)
	shards, rng, remainder = ShardsData(runtime.NumCPU()*4, blocksLen)
	wg.Add(shards)
	for i := 0; i < shards; i++ {
		start, end := ShardsIter(shards, i, rng, remainder)
		go func() {
			transactions := bp.processTxShard(blocks, start, end, &wg)
			mu.Lock()
			txs = append(txs, transactions...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	return
}

// processTxShard processes all transactions over specified range of blocks.
// Creates all send and receive transactions in the range. This func is
// thread safe.
func (bp *ChainPlugin) processTxShard(blocks []*ChainBlock, start, end int, wg *sync.WaitGroup) (txs []*Tx) {
	blocksLen := len(blocks)
	if start >= blocksLen {
		return
	}
	if end > blocksLen {
		end = blocksLen
	}

	var cacheSendTxs []*Tx
	var cacheReceiveTxs []*Tx

	bp.mu.RLock()
	bls := blocks[start:end]
	for i, block := range bls {
		if i%10000 == 0 && IsShuttingDown() {
			break
		}
		lsend, lreceive := ProcessTransactions(bp, block.Block().Header.Timestamp, block.Block().Transactions, bp.txIndex)
		cacheSendTxs = append(cacheSendTxs, lsend...)
		cacheReceiveTxs = append(cacheReceiveTxs, lreceive...)
	}
	bp.mu.RUnlock()

	// Consolidate transactions and add to cache
	txs = bp.processTxs(cacheSendTxs, cacheReceiveTxs)

	wg.Done()
	return
}

// processTxs processes and consolidates send and receive transactions.
func (bp *ChainPlugin) processTxs(sendTxs []*Tx, receiveTxs []*Tx) (txs []*Tx) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Sends
	for _, cacheTx := range sendTxs {
		AddAddrToCache(bp.txCache, cacheTx)
		txs = append(txs, cacheTx)
	}

	// Receives
	// Consolidate payments to self. look for send transactions in
	// the same tx as receive and combine by offsetting send amount
	// and discarding receive record.
	for _, cacheTx := range receiveTxs {
		if sendTx, ok := bp.txCache[cacheTx.Address][cacheTx.KeyCategory(cacheTx.Txid, -1, "send")]; ok {
			sendTx.Amount -= cacheTx.Amount
		} else {
			AddAddrToCache(bp.txCache, cacheTx)
			txs = append(txs, cacheTx)
		}
	}

	return
}

type ChainBlock struct {
	block  *wire.MsgBlock
	hash   chainhash.Hash
	height int64
}

func (b *ChainBlock) Block() *wire.MsgBlock {
	return b.block
}

func (b *ChainBlock) Hash() chainhash.Hash {
	return b.hash
}

func (b *ChainBlock) Height() int64 {
	return b.height
}

func (b *ChainBlock) setHash(hash chainhash.Hash) {
	b.hash = hash
}

// AddAddrToCache adds the tx to the cache. Not thread safe, expects any
// mutexes to be locked outside this call.
func AddAddrToCache(txCache map[string]map[string]*Tx, tx *Tx) {
	if _, ok := txCache[tx.Address]; !ok {
		txCache[tx.Address] = make(map[string]*Tx)
	}
	txCache[tx.Address][tx.Key()] = tx
}

// NewPlugin returns new plugin instance.
func NewPlugin(cfg *chaincfg.Params, blocksDir string, tokenCfg *Token) *ChainPlugin {
	plugin := &ChainPlugin{
		cfg:         cfg,
		blocksDir:   blocksDir,
		isReady:     false,
		txCache:     make(map[string]map[string]*Tx),
		txIndex:     make(map[wire.OutPoint]*BlockTx),
		BlockReader: &chainPluginBlockReader{tokenCfg: tokenCfg},
	}
	if tokenCfg != nil {
		plugin.TokenCfg = tokenCfg
	}
	return plugin
}

// newChainBlock returns a block instance.
func newChainBlock(block *wire.MsgBlock) *ChainBlock {
	newBlock := &ChainBlock{
		block: block,
	}
	return newBlock
}
