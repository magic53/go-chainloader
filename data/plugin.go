package data

import (
	"bufio"
	"bytes"
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

// NewChainBlock returns a block instance.
func NewChainBlock(block *wire.MsgBlock) *ChainBlock {
	newBlock := &ChainBlock{
		block: block,
	}
	return newBlock
}

// PluginTicker returns the ticker symbol (e.g. BLOCK, BTC, LTC).
func PluginTicker(bp Plugin, defaultTicker string) string {
	if !bp.TokenConf().IsNull() {
		return bp.TokenConf().Ticker
	} else {
		return defaultTicker
	}
}

// PluginSegwitActivated returns the segwit activation unix time.
func PluginSegwitActivated(bp Plugin, defaultActivation int64) int64 {
	if !bp.TokenConf().IsNull() {
		return bp.TokenConf().SegwitActivated
	} else {
		return defaultActivation
	}
}

// PluginLoadBlocks load all transactions in the blocks dir.
func PluginLoadBlocks(bp Plugin, blocksDir string) (err error) {
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
		if _, err = bp.ProcessBlocks(bufio.NewReader(fs)); err != nil {
			log.Printf("Error loading block database %s\n", path)
			_ = fs.Close()
			return
		} else {
			log.Printf("%s db file loaded: %s\n", bp.Ticker(), path)
			_ = fs.Close()
		}
	}
	log.Printf("Done loading %s database files\n", bp.Ticker())

	bp.SetReady()
	return
}

// ReadBlock deserializes bytes into block 
func PluginReadBlock(bp Plugin, buf io.ReadSeeker) (block *wire.MsgBlock, err error) {
	var header *wire.BlockHeader
	if header, err = bp.ReadBlockHeader(buf); err != nil {
		log.Println("failed to read block header", err.Error())
		return
	}
	block, err = ReadBlock(buf, header, bp.SegwitActivated())
	return
}

// ReadBlockHeader deserializes block header.
func PluginReadBlockHeader(bp Plugin, buf io.ReadSeeker) (header *wire.BlockHeader, err error) {
	header, err = ReadBlockHeader(buf)
	return
}

// ReadTransaction deserializes bytes into transaction 
func PluginReadTransaction(bp Plugin, buf io.ReadSeeker) (tx *wire.MsgTx, err error) {
	tx, err = ReadTransaction(buf, time.Now().Unix() >= bp.SegwitActivated())
	return
}

// PluginReadBlocks loads block from the reader.
func PluginReadBlocks(bp Plugin, sc *bufio.Reader, network []byte) (blocks []*ChainBlock, err error) {
	_, err = NextBlock(sc, network, func(blockBytes []byte) bool {
		var wireBlock *wire.MsgBlock
		var err2 error
		if wireBlock, err2 = bp.ReadBlock(bytes.NewReader(blockBytes)); err2 != nil {
			log.Println("failed to read block", err2.Error())
			return true
		}
		block := NewChainBlock(wireBlock)
		blocks = append(blocks, block)
		return true
	})

	if err == io.EOF { // not fatal
		err = nil
	}

	return
}

// PluginProcessBlocks will process all blocks in the buffer.
func PluginProcessBlocks(bp Plugin, sc *bufio.Reader) (txs []*Tx, err error) {
	var blocks []*ChainBlock
	if blocks, err = bp.ReadBlocks(sc); err != nil {
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
	bp.Mu().Lock()
	for _, txi := range txBlocks {
		bp.TxIndex()[*txi.OutP] = txi
	}
	bp.Mu().Unlock()

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
			transactions := bp.ProcessTxShard(blocks, start, end, &wg)
			mu.Lock()
			txs = append(txs, transactions...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	return
}

// PluginProcessTxShard processes all transactions over specified range of blocks.
// Creates all send and receive transactions in the range. Thread safe.
func PluginProcessTxShard(bp Plugin, blocks []*ChainBlock, start, end int, wg *sync.WaitGroup) (txs []*Tx) {
	blocksLen := len(blocks)
	if start >= blocksLen {
		return
	}
	if end > blocksLen {
		end = blocksLen
	}

	var cacheSendTxs []*Tx
	var cacheReceiveTxs []*Tx

	bp.Mu().RLock()
	bls := blocks[start:end]
	for i, block := range bls {
		if i%10000 == 0 && IsShuttingDown() {
			break
		}
		lsend, lreceive := ProcessTransactions(bp, block.Block().Header.Timestamp, block.Block().Transactions, bp.TxIndex())
		cacheSendTxs = append(cacheSendTxs, lsend...)
		cacheReceiveTxs = append(cacheReceiveTxs, lreceive...)
	}
	bp.Mu().RUnlock()

	// Consolidate transactions and add to cache
	txs = bp.ProcessTxs(cacheSendTxs, cacheReceiveTxs)

	wg.Done()
	return
}

// PluginProcessTxs processes and consolidates send and receive transactions.
func PluginProcessTxs(bp Plugin, sendTxs []*Tx, receiveTxs []*Tx) (txs []*Tx) {
	bp.Mu().Lock()
	defer bp.Mu().Unlock()

	// Sends
	for _, cacheTx := range sendTxs {
		bp.AddTransactionToCache(cacheTx)
		txs = append(txs, cacheTx)
	}

	// Receives
	// Consolidate payments to self. look for send transactions in
	// the same tx as receive and combine by offsetting send amount
	// and discarding receive record.
	for _, cacheTx := range receiveTxs {
		if sendTx, ok := bp.TxCache()[cacheTx.Address][cacheTx.KeyCategory(cacheTx.Txid, -1, "send")]; ok {
			sendTx.Amount -= cacheTx.Amount
		} else {
			bp.AddTransactionToCache(cacheTx)
			txs = append(txs, cacheTx)
		}
	}

	return
}

// PluginAddAddrToCache adds the tx to the cache. Not thread safe, expects any
// mutexes to be locked outside this call.
func PluginAddAddrToCache(bp Plugin, tx *Tx) {
	if _, ok := bp.TxCache()[tx.Address]; !ok {
		bp.TxCache()[tx.Address] = make(map[string]*Tx)
	}
	bp.TxCache()[tx.Address][tx.Key()] = tx
}

// PluginAddBlocks process new blocks received by the network to keep internal chain data
// up to date.
func PluginAddBlocks(bp Plugin, blocks []byte) (txs []*Tx, err error) {
	r := bytes.NewReader(blocks)
	txs, err = bp.ProcessBlocks(bufio.NewReader(r))
	return
}

// PluginImportTransactions imports the specified transactions into the data store.
func PluginImportTransactions(bp Plugin, transactions []*wire.MsgTx) (txs []*Tx, err error) {
	bp.Mu().RLock()
	sendTxs, receiveTxs := ProcessTransactions(bp, time.Now(), transactions, bp.TxIndex())
	bp.Mu().RUnlock()
	txs = bp.ProcessTxs(sendTxs, receiveTxs)
	return
}

// ProcessTransactions will process all transactions in blocks.
func ProcessTransactions(plugin Plugin, blockTime time.Time, transactions []*wire.MsgTx, txIndex map[wire.OutPoint]*BlockTx) (sendTxs, receiveTxs []*Tx) {
	blockhash := chainhash.Hash{} // TODO block.BlockHash()
	cfg := plugin.Config()
	for _, tx := range transactions {
		txHash := tx.TxHash()
		txHashStr := txHash.String()

		// Send category
		for _, vin := range tx.TxIn {
			prevoutTx, ok := txIndex[vin.PreviousOutPoint]
			if !ok {
				continue
			}
			prevTx := prevoutTx.Transaction
			scriptPk := prevTx.TxOut[vin.PreviousOutPoint.Index].PkScript
			amount := float64(prevTx.TxOut[vin.PreviousOutPoint.Index].Value) / 100000000.0 // TODO Assumes coin denomination is 100M
			confirmations := 0                                                              // TODO Confirmations for send transaction
			blockhash := blockhash
			outp := &vin.PreviousOutPoint
			sendTxs = append(sendTxs, ExtractAddresses(scriptPk, txHashStr, -1, amount,
				blockTime, confirmations, "send", &blockhash, outp, &cfg)...)
		}

		// Receive category
		for i, vout := range tx.TxOut {
			scriptPk := vout.PkScript
			amount := float64(vout.Value) / 100000000.0 // TODO Assumes coin denomination is 100M
			confirmations := 0                          // TODO Confirmations for send transaction
			blockhash := blockhash
			outp := wire.NewOutPoint(&txHash, uint32(i))
			receiveTxs = append(receiveTxs, ExtractAddresses(scriptPk, txHashStr, i, amount,
				blockTime, confirmations, "receive", &blockhash, outp, &cfg)...)
		}
	}
	return
}
