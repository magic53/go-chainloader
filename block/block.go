package block

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/blocknetdx/go-exrplugins/data"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"sync"
)

type Plugin struct {
	mu        sync.RWMutex
	cfg       *chaincfg.Params
	blocksDir string
	isReady   bool
	txIndex   map[wire.OutPoint]*data.BlockTx
	txCache   map[string]map[string]*data.Tx
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
	exists := false
	if exists, err = data.FileExists(blocksDir); err != nil || !exists {
		if !exists {
			err = errors.New(fmt.Sprintf("File doesn't exist: %s", blocksDir))
		}
		return
	}

	var files []os.FileInfo
	if files, err = ioutil.ReadDir(blocksDir); err != nil {
		return
	}
	if len(files) == 0 { // check if no block files
		err = errors.New("BLOCK no db files found")
		return
	}

	// Filter out all non blk files
	var filterFiles []os.FileInfo
	re := regexp.MustCompile(`blk(\d+).dat`)
	for _, file := range files {
		if !re.MatchString(file.Name()) {
			continue
		}
		filterFiles = append(filterFiles, file)
	}

	// Sort most recent first
	sort.Slice(filterFiles, func(a, b int) bool {
		aName := filterFiles[a].Name()
		bName := filterFiles[b].Name()
		aMatches := re.FindStringSubmatch(aName)
		if len(aMatches) < 1 {
			return false
		}
		bMatches := re.FindStringSubmatch(bName)
		if len(bMatches) < 1 {
			return false
		}
		var err2 error
		var ai int64
		if ai, err2 = strconv.ParseInt(aMatches[1], 10, 64); err2 != nil {
			return true
		}
		var bi int64
		if bi, err2 = strconv.ParseInt(bMatches[1], 10, 64); err2 != nil {
			return false
		}
		return ai > bi // sort descending
	})

	// TODO Limit loading the number of blk files
	dats := 3 // len(filterFiles)
	if len(filterFiles) >= dats {
		filterFiles = filterFiles[:dats]
	} else {
		filterFiles = filterFiles[:1]
	}

	// Iterate oldest blocks first (filterFiles sorted descending)
	for _, file := range filterFiles {
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

	bp.setReady()
	return
}

// LoadBlocks loads block from the reader.
func (bp *Plugin) loadBlocks(sc *bufio.Reader, network []byte) (blocks []*BLOCK, txIndex []*data.BlockTx, err error) {
	_, err = data.NextBlock(sc, network, func(blockBytes []byte) bool {
		var wireBlock *wire.MsgBlock
		var err2 error
		if wireBlock, err2 = bp.ReadBlock(bytes.NewReader(blockBytes)); err2 != nil {
			log.Println("failed to read block", err2.Error())
			return true
		}
		block := newBlocknetBlock(wireBlock)
		blocks = append(blocks, block)
		for _, tx := range wireBlock.Transactions {
			for n := range tx.TxOut {
				txHash := tx.TxHash()
				outp := wire.NewOutPoint(&txHash, uint32(n))
				txIndex = append(txIndex, &data.BlockTx{
					OutP:        outp,
					Transaction: tx,
				})
			}
		}
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
	block, err = data.ReadBlock(buf, header, 1584537260)
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

// AddBlocks process new blocks received by the network to keep internal chain data
// up to date.
func (bp *Plugin) AddBlocks(blocks []byte) (txs []*data.Tx, err error) {
	r := bytes.NewReader(blocks)
	txs, err = bp.processBlocks(bufio.NewReader(r))
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
	var blocks []*BLOCK
	var txBlocks []*data.BlockTx
	if blocks, txBlocks, err = bp.loadBlocks(sc, data.NetworkLE(bp.Network())); err != nil {
		log.Println("Error loading block database")
		return
	}

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
	blocksLen := len(blocks)
	shards, rng, remainder := data.ShardsData(runtime.NumCPU()*4, blocksLen)
	var wg sync.WaitGroup
	wg.Add(shards)
	mu := sync.Mutex{}
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
func (bp *Plugin) processTxShard(blocks []*BLOCK, start, end int, wg *sync.WaitGroup) (txs []*data.Tx) {
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
	for _, block := range bls {
		lsend, lreceive := data.ProcessTransactions(bp, block.Block(), block.Block().Transactions, bp.txIndex)
		cacheSendTxs = append(cacheSendTxs, lsend...)
		cacheReceiveTxs = append(cacheReceiveTxs, lreceive...)
	}
	bp.mu.RUnlock()

	bp.mu.Lock()
	// Sends
	for _, cacheTx := range cacheSendTxs {
		addAddrToCache(bp.txCache, cacheTx)
		txs = append(txs, cacheTx)
	}

	// Receives
	// Consolidate payments to self. look for send transactions in
	// the same tx as receive and combine by offsetting send amount
	// and discarding receive record.
	for _, cacheTx := range cacheReceiveTxs {
		if sendTx, ok := bp.txCache[cacheTx.Address][cacheTx.KeyCategory(cacheTx.Txid, -1, "send")]; ok {
			sendTx.Amount -= cacheTx.Amount
		} else {
			addAddrToCache(bp.txCache, cacheTx)
			txs = append(txs, cacheTx)
		}
	}
	bp.mu.Unlock()

	wg.Done()
	return
}

type BLOCK struct {
	block  *wire.MsgBlock
	hash   chainhash.Hash
	height int64
}

func (b *BLOCK) Block() *wire.MsgBlock {
	return b.block
}

func (b *BLOCK) Hash() chainhash.Hash {
	return b.hash
}

func (b *BLOCK) Height() int64 {
	return b.height
}

func (b *BLOCK) setHash(hash chainhash.Hash) {
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
func NewPlugin(cfg *chaincfg.Params, blocksDir string) data.Plugin {
	plugin := &Plugin{
		cfg:       cfg,
		blocksDir: blocksDir,
		isReady:   false,
		txCache:   make(map[string]map[string]*data.Tx),
		txIndex:   make(map[wire.OutPoint]*data.BlockTx),
	}
	return plugin
}

// newBlocknetBlock returns a block instance.
func newBlocknetBlock(block *wire.MsgBlock) *BLOCK {
	newBlock := &BLOCK{
		block: block,
	}
	return newBlock
}
