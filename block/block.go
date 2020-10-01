package block

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/blocknetdx/go-exrplugins/data"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
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
	"time"
)

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

type Plugin struct {
	mu        sync.RWMutex
	blocksDir string
	isReady   bool
	network   wire.BitcoinNet
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

// ClearIndex removes references to the transaction index. This typically frees up
// significant memory.
func (bp *Plugin) ClearIndex() {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.txIndex = map[wire.OutPoint]*data.BlockTx{}
}

// LoadBlocks load all BLOCK transactions in the blocksDir.
func (bp *Plugin) LoadBlocks(blocksDir string, cfg *chaincfg.Params) (err error) {
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

	// network magic number to use when reading block db
	network := data.NetworkLE(bp.network)

	// Iterate oldest blocks first (filterFiles sorted descending)
	for _, file := range filterFiles {
		path := filepath.Join(blocksDir, file.Name())
		var fs *os.File
		fs, err = os.Open(path)
		if err != nil {
			return
		}
		var curBlocks []*BLOCK
		var txIndex []*data.BlockTx
		if curBlocks, txIndex, err = bp.loadBlocks(bufio.NewReader(fs), network); err != nil {
			log.Println("Error loading block database")
			_ = fs.Close()
			return
		} else {
			log.Printf("BLOCK db file loaded: %s\n", path)
			_ = fs.Close()
		}

		// Update the tx index
		bp.mu.Lock()
		for _, txi := range txIndex {
			bp.txIndex[*txi.OutP] = txi
		}
		bp.mu.Unlock()

		// Sort blocks by time ascending
		sort.Slice(curBlocks, func(i, j int) bool {
			return curBlocks[i].Block().Header.Timestamp.Unix() < curBlocks[j].Block().Header.Timestamp.Unix()
		})

		// Process transactions, spawn goroutines to process all current blocks
		// Use a multiplier of num cpu as a starting point, let go scheduler fill
		// in work.
		blocksLen := len(curBlocks)
		shards, rng, remainder := data.ShardsData(runtime.NumCPU()*4, blocksLen)
		var wg sync.WaitGroup
		wg.Add(shards)
		for i := 0; i < shards; i++ {
			start, end := data.ShardsIter(shards, i, rng, remainder)
			go bp.processTxShard(curBlocks, start, end, cfg, &wg)
		}
		wg.Wait()
	}

	bp.ClearIndex()
	bp.setReady()
	return
}

// LoadBlocks loads block from the reader.
func (bp *Plugin) loadBlocks(sc *bufio.Reader, network []byte) (blocks []*BLOCK, txIndex []*data.BlockTx, err error) {
	for err == nil && sc.Size() > 80 {
		var b byte
		if b, err = sc.ReadByte(); err != nil {
			if err != io.EOF {
				log.Println("failed to read byte", err.Error())
			}
			break
		}

		if !bytes.Equal([]byte{b}, network[:1]) { // check for network byte delim
			continue
		}
		var pb []byte
		if pb, err = sc.Peek(3); err != nil { // peek network bytes
			log.Println("failed to peek", err.Error())
			continue
		}
		fpb := append([]byte{b}, pb...)
		if !bytes.Equal(fpb, network) { // check if network matches
			continue
		}

		// We're at the block, discard network magic number bytes
		if _, err = sc.Discard(3); err != nil {
			log.Println("failed to discard bytes", err.Error())
			continue
		}

		// read the block size
		sizeBytes := make([]byte, 4)
		if _, err = io.ReadFull(sc, sizeBytes); err != nil {
			log.Println("failed to read block size", err.Error())
			continue
		}
		size := int(binary.LittleEndian.Uint32(sizeBytes))

		// copy block bytes into buffer for processing later
		blockBytes := make([]byte, size)
		if n, err2 := io.ReadFull(sc, blockBytes); err2 != nil {
			if err2 == io.EOF {
				break
			}
			log.Println("failed to copy block bytes", err2.Error())
			if size-n > 0 { // Skip bytes
				_, _ = sc.Discard(size - n)
			}
			continue
		}

		var wireBlock *wire.MsgBlock
		var bytesNotRead int // block size
		var err2 error
		if wireBlock, bytesNotRead, err2 = bp.readBlock(bytes.NewReader(blockBytes)); err2 != nil {
			log.Println("failed to read block", err2.Error())
			if bytesNotRead > 0 { // Skip bytes
				_, _ = sc.Discard(bytesNotRead)
			}
			continue
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
	}

	if err == io.EOF { // not fatal
		err = nil
	}

	return
}

// readBlock deserializes bytes into block data.
func (bp *Plugin) readBlock(buf io.ReadSeeker) (block *wire.MsgBlock, size int, err error) {
	var header *wire.BlockHeader
	if header, err = bp.readBlockHeader(buf); err != nil {
		log.Println("failed to read block header", err.Error())
		return
	}
	block = &wire.MsgBlock{
		Header:       *header,
		Transactions: []*wire.MsgTx{},
	}

	// Deserialize transactions
	var txLen uint64
	if txLen, err = wire.ReadVarInt(buf, 0); err != nil {
		log.Println("failed to read tx count", err.Error())
		return
	}

	// Iterate over all transactions
	for i := 0; i < int(txLen); i++ {
		var vins []*wire.TxIn
		var vouts []*wire.TxOut

		txVersionB := make([]byte, 4)
		if _, err = io.ReadFull(buf, txVersionB); err != nil {
			log.Println("failed to read tx version", err.Error())
			return
		}
		txVersion := binary.LittleEndian.Uint32(txVersionB)

		txAllowWitness := header.Timestamp.Unix() >= 1584537260 //header.Version & 0x40000000 == 0
		if txAllowWitness {
			txWitnessMarker := make([]byte, 2)
			if _, err = io.ReadFull(buf, txWitnessMarker); err != nil {
				log.Println("failed to read tx vins witness marker", err.Error())
				return
			}
			if bytes.Equal(txWitnessMarker, []byte{0x0, 0x1}) {
				var vinLen uint64
				if vins, vinLen, err = data.ReadVins(buf); err != nil {
					log.Println("failed to read tx vins 2", err.Error())
					return
				}
				if vouts, _, err = data.ReadVouts(buf); err != nil {
					log.Println("failed to read tx vouts 2", err.Error())
					return
				}
				// process witness data
				for i := 0; i < int(vinLen); i++ {
					var witLen uint64
					if witLen, err = wire.ReadVarInt(buf, 0); err != nil {
						log.Println("failed to read tx witness data length", err.Error())
						return
					}
					for j := 0; j < int(witLen); j++ {
						var jwitLen uint64
						if jwitLen, err = wire.ReadVarInt(buf, 0); err != nil {
							log.Println("failed to read tx witness data length 2", err.Error())
							return
						}
						// TODO Currently ignoring witness data
						// discard bytes
						if _, err = buf.Seek(int64(jwitLen), io.SeekCurrent); err != nil {
							log.Println("failed to discard tx witness data", err.Error())
							return
						}
					}
				}
			} else {
				// reset the buffer prior to witness marker check
				if _, err = buf.Seek(-2, io.SeekCurrent); err != nil {
					log.Println("failed to reset tx vin dummy", err.Error())
					return
				}
				if vins, _, err = data.ReadVins(buf); err != nil {
					log.Println("failed to read tx vins 2", err.Error())
					return
				}
				if vouts, _, err = data.ReadVouts(buf); err != nil {
					log.Println("failed to read tx vouts 2", err.Error())
					return
				}
			}
		} else { // no witness
			if vins, _, err = data.ReadVins(buf); err != nil {
				log.Println("failed to read tx vins 2", err.Error())
				return
			}
			if vouts, _, err = data.ReadVouts(buf); err != nil {
				log.Println("failed to read tx vouts 2", err.Error())
				return
			}
		}

		// Locktime
		txLockTimeB := make([]byte, 4)
		if _, err = io.ReadFull(buf, txLockTimeB); err != nil {
			log.Println("failed to read tx locktime", err.Error())
			return
		}
		txLockTime := binary.LittleEndian.Uint32(txLockTimeB)

		tx := &wire.MsgTx{
			Version:  int32(txVersion),
			TxIn:     vins,
			TxOut:    vouts,
			LockTime: txLockTime,
		}
		block.Transactions = append(block.Transactions, tx)
	}

	return
}

// readBlockHeader deserializes block header.
func (bp *Plugin) readBlockHeader(buf io.ReadSeeker) (header *wire.BlockHeader, err error) {
	versionB := make([]byte, 4)
	prevBlockB := make([]byte, 32)
	merkleB := make([]byte, 32)
	blockTimeB := make([]byte, 4)
	bitsB := make([]byte, 4)
	nonceB := make([]byte, 4)
	hashStakeLen := 32
	stakeIndexLen := 4
	stakeAmountLen := 8
	hashStakeBlockLen := 32
	stakingProtocolLen := int64(hashStakeLen + stakeIndexLen + stakeAmountLen + hashStakeBlockLen)

	if _, err = io.ReadFull(buf, versionB); err != nil {
		return
	}
	if _, err = io.ReadFull(buf, prevBlockB); err != nil {
		return
	}
	if _, err = io.ReadFull(buf, merkleB); err != nil {
		return
	}
	if _, err = io.ReadFull(buf, blockTimeB); err != nil {
		return
	}
	if _, err = io.ReadFull(buf, bitsB); err != nil {
		return
	}
	if _, err = io.ReadFull(buf, nonceB); err != nil {
		return
	}
	// Read beyond staking protocol fields
	if _, err = buf.Seek(stakingProtocolLen, io.SeekCurrent); err != nil {
		return
	}

	version := int32(binary.LittleEndian.Uint32(versionB))
	var prevBlock *chainhash.Hash
	if prevBlock, err = chainhash.NewHash(prevBlockB); err != nil {
		return
	}
	var merkle *chainhash.Hash
	if merkle, err = chainhash.NewHash(merkleB); err != nil {
		return
	}
	blockTime := binary.LittleEndian.Uint32(blockTimeB)
	bits := binary.LittleEndian.Uint32(bitsB)
	nonce := binary.LittleEndian.Uint32(nonceB)

	header = wire.NewBlockHeader(version, prevBlock, merkle, bits, nonce)
	header.Timestamp = time.Unix(int64(blockTime), 0)
	return
}

// setReady state on the plugin.
func (bp *Plugin) setReady() {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.isReady = true
}

// processTxShard processes all transactions over specified range of blocks.
// Creates all send and receive transactions in the range. This func is
// thread safe.
func (bp *Plugin) processTxShard(blocks []*BLOCK, start, end int, cfg *chaincfg.Params, wg *sync.WaitGroup) {
	var cacheSendTxs []*data.Tx
	var cacheReceiveTxs []*data.Tx

	bp.mu.RLock()
	for i := start; i < end; i++ {
		bloc := blocks[i]
		for _, tx := range bloc.Block().Transactions {
			txHash := tx.TxHash()
			txHashStr := txHash.String()

			// Send category
			for _, vin := range tx.TxIn {
				prevoutTx, ok := bp.txIndex[vin.PreviousOutPoint]
				if !ok {
					continue
				}
				prevTx := prevoutTx.Transaction
				scriptPk := prevTx.TxOut[vin.PreviousOutPoint.Index].PkScript
				amount := float64(prevTx.TxOut[vin.PreviousOutPoint.Index].Value) / 100000000.0 // TODO Assumes coin denomination is 100M
				confirmations := 0                                                              // TODO Confirmations for send transaction
				blockhash := bloc.Hash()
				outp := &vin.PreviousOutPoint
				cacheSendTxs = append(cacheSendTxs, extractTxs(scriptPk, txHashStr, -1, amount,
					bloc.Block().Header.Timestamp, confirmations, "send", &blockhash, outp, cfg)...)
			}

			// Receive category
			for i, vout := range tx.TxOut {
				scriptPk := vout.PkScript
				amount := float64(vout.Value) / 100000000.0 // TODO Assumes coin denomination is 100M
				confirmations := 0                          // TODO Confirmations for send transaction
				blockhash := bloc.Hash()
				outp := wire.NewOutPoint(&txHash, uint32(i))
				cacheReceiveTxs = append(cacheReceiveTxs, extractTxs(scriptPk, txHashStr, i, amount,
					bloc.Block().Header.Timestamp, confirmations, "receive", &blockhash, outp, cfg)...)
			}
		}
	}
	bp.mu.RUnlock()

	bp.mu.Lock()
	// Sends
	for _, cacheTx := range cacheSendTxs {
		addAddrToCache(bp.txCache, cacheTx)
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
		}
	}
	bp.mu.Unlock()

	wg.Done()
}

// addAddrToCache adds the tx to the cache. Not thread safe, expects any
// mutexes to be locked outside this call.
func addAddrToCache(txCache map[string]map[string]*data.Tx, tx *data.Tx) {
	if _, ok := txCache[tx.Address]; !ok {
		txCache[tx.Address] = make(map[string]*data.Tx)
	}
	txCache[tx.Address][tx.Key()] = tx
}

// extractTxs derives transactions from scriptPubKey.
func extractTxs(scriptPk []byte, txHash string, txVout int, amount float64, blockTime time.Time, confirmations int,
	category string, blockHash *chainhash.Hash, outp *wire.OutPoint, cfg *chaincfg.Params) (txs []*data.Tx) {
	var addrs []btcutil.Address
	var err error
	if _, addrs, _, err = txscript.ExtractPkScriptAddrs(scriptPk, cfg); err != nil {
		return
	}
	// TODO Support additional address types (bech32, p2sh)
	for _, addr := range addrs {
		if addr == nil {
			continue
		}
		switch addr := addr.(type) {
		case *btcutil.AddressPubKeyHash:
			address := addr.EncodeAddress()
			cacheTx := &data.Tx{
				Txid:          txHash,
				Vout:          int32(txVout),
				Address:       address,
				Category:      category,
				Amount:        amount,
				Time:          blockTime.Unix(),
				Confirmations: uint32(confirmations), // TODO Confirmations
				Blockhash:     blockHash,
				OutP:          outp,
			}
			txs = append(txs, cacheTx)
		}
	}
	return
}

// NewPlugin returns new BLOCK plugin instance.
func NewPlugin(blocksDir string) data.BlockLoader {
	plugin := &Plugin{
		blocksDir: blocksDir,
		isReady:   false,
		network:   wire.MainNet,
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
