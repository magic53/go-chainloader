package block

import (
	"bufio"
	"bytes"
	"github.com/blocknetdx/go-exrplugins/data"
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

type Plugin struct {
	mu        sync.RWMutex
	cfg       *chaincfg.Params
	blocksDir string
	isReady   bool
	txIndex   map[wire.OutPoint]*data.BlockTx
	txCache   map[string]map[string]*data.Tx
	tokenCfg  *data.Token
}

// SegwitActivated returns the segwit activation unix time.
func (bp *Plugin) SegwitActivated() int64 {
	if bp.tokenCfg != nil {
		return bp.tokenCfg.SegwitActivated
	} else {
		return 1584537260 // unix time
	}
}

// BlocksDir returns the location of all block dat files.
func (bp *Plugin) BlocksDir() string {
	return bp.blocksDir
}

// Ready returns true if the block db has loaded.
func (bp *Plugin) Ready() bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.isReady
}

// Network returns the network magic number.
func (bp *Plugin) Network() wire.BitcoinNet {
	return bp.cfg.Net
}

// Config returns the network magic number.
func (bp *Plugin) Config() chaincfg.Params {
	return *bp.cfg
}

// ClearIndex removes references to the transaction index. This typically frees up
// significant memory.
func (bp *Plugin) ClearIndex() {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.txIndex = map[wire.OutPoint]*data.BlockTx{}
}

// LoadBlocks load all BLOCK transactions in the blocksDir.
func (bp *Plugin) LoadBlocks(blocksDir string) (err error) {
	var files []os.FileInfo
	files, err = data.BlockFiles(blocksDir, regexp.MustCompile(`blk(\d+).dat`), 3)

	// Iterate oldest blocks first (filterFiles sorted descending)
	for _, file := range files {
		if data.IsShuttingDown() {
			return
		}
		path := filepath.Join(blocksDir, file.Name())
		var fs *os.File
		fs, err = os.Open(path)
		if err != nil {
			return
		}
		if _, err = bp.processBlocks(bufio.NewReader(fs)); err != nil {
			log.Println("Error loading block database")
			_ = fs.Close()
			return
		} else {
			log.Printf("BLOCK db file loaded: %s\n", path)
			_ = fs.Close()
		}
	}
	log.Println("Done loading BLOCK database files")

	bp.setReady()
	return
}

// loadBlocks loads block from the reader.
func (bp *Plugin) loadBlocks(sc *bufio.Reader, network []byte) (blocks []*Block, err error) {
	_, err = data.NextBlock(sc, network, func(blockBytes []byte) bool {
		var wireBlock *wire.MsgBlock
		var err2 error
		if wireBlock, err2 = bp.ReadBlock(bytes.NewReader(blockBytes)); err2 != nil {
			log.Println("failed to read block", err2.Error())
			return true
		}
		block := newBlocknetBlock(wireBlock)
		blocks = append(blocks, block)
		return true
	})

	if err == io.EOF { // not fatal
		err = nil
	}

	return
}

// ReadBlock deserializes bytes into block data.
func (bp *Plugin) ReadBlock(buf io.ReadSeeker) (block *wire.MsgBlock, err error) {
	var header *wire.BlockHeader
	if header, err = bp.ReadBlockHeader(buf); err != nil {
		log.Println("failed to read block header", err.Error())
		return
	}
	block, err = data.ReadBlock(buf, header, bp.SegwitActivated())
	return
}

// ReadBlockHeader deserializes block header.
func (bp *Plugin) ReadBlockHeader(buf io.ReadSeeker) (header *wire.BlockHeader, err error) {
	header, err = data.ReadBlockHeader(buf)

	hashStakeLen := 32
	stakeIndexLen := 4
	stakeAmountLen := 8
	hashStakeBlockLen := 32
	stakingProtocolLen := int64(hashStakeLen + stakeIndexLen + stakeAmountLen + hashStakeBlockLen)

	// Skip over staking protocol fields
	if _, err = buf.Seek(stakingProtocolLen, io.SeekCurrent); err != nil {
		log.Println("failed to seek beyond block header staking protocol bytes")
		return
	}

	return
}

// ReadTransaction deserializes bytes into transaction data.
func (bp *Plugin) ReadTransaction(buf io.ReadSeeker) (tx *wire.MsgTx, err error) {
	tx, err = data.ReadTransaction(buf, time.Now().Unix() >= bp.SegwitActivated())
	return
}

// AddBlocks process new blocks received by the network to keep internal chain data
// up to date.
func (bp *Plugin) AddBlocks(blocks []byte) (txs []*data.Tx, err error) {
	r := bytes.NewReader(blocks)
	txs, err = bp.processBlocks(bufio.NewReader(r))
	return
}

// ImportTransactions imports the specified transactions into the data store.
func (bp *Plugin) ImportTransactions(transactions []*wire.MsgTx) (txs []*data.Tx, err error) {
	bp.mu.RLock()
	sendTxs, receiveTxs := data.ProcessTransactions(bp, time.Now(), transactions, bp.txIndex)
	bp.mu.RUnlock()
	txs = bp.processTxs(sendTxs, receiveTxs)
	return
}

// setReady state on the plugin.
func (bp *Plugin) setReady() {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.isReady = true
}

// processBlocks will process all blocks in the buffer.
func (bp *Plugin) processBlocks(sc *bufio.Reader) (txs []*data.Tx, err error) {
	var blocks []*Block
	if blocks, err = bp.loadBlocks(sc, data.NetworkLE(bp.Network())); err != nil {
		log.Println("Error loading block database")
		return
	}

	// Produce block transaction index
	var txBlocks []*data.BlockTx
	blocksLen := len(blocks)
	shards, rng, remainder := data.ShardsData(runtime.NumCPU()*4, blocksLen)
	var wg sync.WaitGroup
	wg.Add(shards)
	mu := sync.Mutex{}
	for i := 0; i < shards; i++ {
		start, end := data.ShardsIter(shards, i, rng, remainder)
		go func() {
			var transactions []*data.BlockTx
			for j := start; j < end; j++ {
				wireBlock := blocks[j].Block()
				for _, tx := range wireBlock.Transactions {
					txHash := tx.TxHash()
					for n := range tx.TxOut {
						outp := wire.NewOutPoint(&txHash, uint32(n))
						transactions = append(transactions, &data.BlockTx{
							OutP:        outp,
							Transaction: tx,
						})
					}
				}
				if j % 1000 == 0 && data.IsShuttingDown() {
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
	shards, rng, remainder = data.ShardsData(runtime.NumCPU()*4, blocksLen)
	wg.Add(shards)
	for i := 0; i < shards; i++ {
		start, end := data.ShardsIter(shards, i, rng, remainder)
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
func (bp *Plugin) processTxShard(blocks []*Block, start, end int, wg *sync.WaitGroup) (txs []*data.Tx) {
	blocksLen := len(blocks)
	if start >= blocksLen {
		return
	}
	if end > blocksLen {
		end = blocksLen
	}

	var cacheSendTxs []*data.Tx
	var cacheReceiveTxs []*data.Tx

	bp.mu.RLock()
	bls := blocks[start:end]
	for i, block := range bls {
		if i % 10000 == 0 && data.IsShuttingDown() {
			break
		}
		lsend, lreceive := data.ProcessTransactions(bp, block.Block().Header.Timestamp, block.Block().Transactions, bp.txIndex)
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
func (bp *Plugin) processTxs(sendTxs []*data.Tx, receiveTxs []*data.Tx) (txs []*data.Tx) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Sends
	for _, cacheTx := range sendTxs {
		addAddrToCache(bp.txCache, cacheTx)
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
			addAddrToCache(bp.txCache, cacheTx)
			txs = append(txs, cacheTx)
		}
	}

	return
}

type Block struct {
	block  *wire.MsgBlock
	hash   chainhash.Hash
	height int64
}

func (b *Block) Block() *wire.MsgBlock {
	return b.block
}

func (b *Block) Hash() chainhash.Hash {
	return b.hash
}

func (b *Block) Height() int64 {
	return b.height
}

func (b *Block) setHash(hash chainhash.Hash) {
	b.hash = hash
}

// addAddrToCache adds the tx to the cache. Not thread safe, expects any
// mutexes to be locked outside this call.
func addAddrToCache(txCache map[string]map[string]*data.Tx, tx *data.Tx) {
	if _, ok := txCache[tx.Address]; !ok {
		txCache[tx.Address] = make(map[string]*data.Tx)
	}
	txCache[tx.Address][tx.Key()] = tx
}

// NewPlugin returns new BLOCK plugin instance.
func NewPlugin(cfg *chaincfg.Params, blocksDir string, tokenCfg *data.Token) data.Plugin {
	plugin := &Plugin{
		cfg:       cfg,
		blocksDir: blocksDir,
		isReady:   false,
		txCache:   make(map[string]map[string]*data.Tx),
		txIndex:   make(map[wire.OutPoint]*data.BlockTx),
	}
	if tokenCfg != nil {
		plugin.tokenCfg = tokenCfg
	}
	return plugin
}

// newBlocknetBlock returns a block instance.
func newBlocknetBlock(block *wire.MsgBlock) *Block {
	newBlock := &Block{
		block: block,
	}
	return newBlock
}
