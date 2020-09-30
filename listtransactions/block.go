package listtransactions

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
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
	"sort"
	"strconv"
	"time"
)

type BLOCK struct {
	block *wire.MsgBlock
	hash chainhash.Hash
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

type BLOCKPlugin struct {
	blocksDir string
	isReady   bool
	network   wire.BitcoinNet
	blocks    []*BLOCK
	txIndex   map[wire.OutPoint]*BlockTx
	txCache   map[string]map[string]*Tx
}

// BlocksDir returns the location of all block dat files.
func (bp *BLOCKPlugin) BlocksDir() string {
	return bp.blocksDir
}

// Ready returns true if the block db has loaded.
func (bp *BLOCKPlugin) Ready() bool {
	return bp.isReady
}

// LoadBlocks load all BLOCK transactions in the blocksDir.
func (bp *BLOCKPlugin) LoadBlocks(blocksDir string) (err error) {
	exists := false
	if exists, err = fileExists(blocksDir); err != nil || !exists {
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
	//dats := 2 // len(filterFiles)
	//if len(filterFiles) >= dats {
	//	filterFiles = filterFiles[:dats]
	//} else {
	//	filterFiles = filterFiles[:1]
	//}
	filterFiles = filterFiles[:2]

	// network magic number to use when reading block db
	network := networkLE(bp.network)
	// Iterate oldest blocks first (filterFiles sorted descending)
	for _, file := range filterFiles {
		path := filepath.Join(blocksDir, file.Name())
		var fs *os.File
		fs, err = os.Open(path)
		if err != nil {
			return
		}
		defer fs.Close()
		var curBlocks []*BLOCK
		if curBlocks, err = bp.loadBlocks(bufio.NewReader(fs), network); err != nil {
			log.Println("Error loading block database")
			return
		} else {
			log.Printf("BLOCK db file loaded: %s\n", path)
		}
		// update tx index
		for _, bloc := range curBlocks {
			for _, tx := range bloc.Block().Transactions {
				for n, _ := range tx.TxOut {
					outp := wire.OutPoint{Hash: tx.TxHash(), Index: uint32(n)}
					bp.txIndex[outp] = &BlockTx{
						Block: bloc,
						Transaction: tx,
					}
				}
			}
		}
		bp.blocks = append(bp.blocks, curBlocks...)
	}

	sort.Slice(bp.blocks, func(i, j int) bool {
		return bp.blocks[i].Block().Header.Timestamp.Unix() < bp.blocks[j].Block().Header.Timestamp.Unix()
	})

	// Cache transactions
	for _, bloc := range bp.blocks {
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
				var addrs []btcutil.Address
				if _, addrs, _, err = txscript.ExtractPkScriptAddrs(scriptPk, &chaincfg.MainNetParams); err != nil {
					continue
				}
				// TODO Support additional address types (bech32, p2sh)
				for _, addr := range addrs {
					if addr == nil {
						continue
					}
					switch addr := addr.(type) {
					case *btcutil.AddressPubKeyHash:
						address := addr.EncodeAddress()
						cacheTx := &Tx{
							Txid:          txHashStr,
							Vout:          -1,
							Address:       address,
							Category:      "send",
							Amount:        float64(prevTx.TxOut[vin.PreviousOutPoint.Index].Value) / 100000000.0, // TODO Assumes coin denomination is 100M
							Time:          bloc.Block().Header.Timestamp.Unix(),
							Confirmations: 0, // TODO Confirmations
							Blockhash:     bloc.Hash(),
							OutP:		   vin.PreviousOutPoint,
						}
						addAddrToCache(bp.txCache, cacheTx)
					}
				}
			}
			// Receive category
			for i, vout := range tx.TxOut {
				scriptPk := vout.PkScript
				var addrs []btcutil.Address
				if _, addrs, _, err = txscript.ExtractPkScriptAddrs(scriptPk, &chaincfg.MainNetParams); err != nil {
					continue
				}
				// TODO Support additional address types (bech32, p2sh)
				for _, addr := range addrs {
					if addr == nil {
						continue
					}
					switch addr := addr.(type) {
					case *btcutil.AddressPubKeyHash:
						address := addr.EncodeAddress()
						cacheTx := &Tx{
							Txid:          txHashStr,
							Vout:          int32(i),
							Address:       address,
							Category:      "receive",
							Amount:        float64(vout.Value) / 100000000.0, // TODO Assumes coin denomination is 100M
							Time:          bloc.Block().Header.Timestamp.Unix(),
							Confirmations: 0, // TODO Confirmations
							Blockhash:     bloc.Hash(),
							OutP:		   *wire.NewOutPoint(&txHash, uint32(i)),
						}
						addAddrToCache(bp.txCache, cacheTx)
					}
				}
			}
		}
	}

	bp.isReady = true
	return
}

// addAddrToCache adds the
func addAddrToCache(txCache map[string]map[string]*Tx, tx *Tx) {
	if _, ok := txCache[tx.Address]; !ok {
		txCache[tx.Address] = make(map[string]*Tx)
	}
	txCache[tx.Address][tx.Key()] = tx
}

// ListTransactions returns BLOCK transactions between fromTime and toTime.
func (bp *BLOCKPlugin) ListTransactions(fromTime, toTime int64, addresses []string) (txs []*Tx, err error) {
	for _, address := range addresses {
		transactions, ok := bp.txCache[address]
		if !ok {
			continue
		}
		for _, tx := range transactions {
			if tx.Time >= fromTime && tx.Time <= toTime {
				txs = append(txs, tx)
			}
		}
	}
 	return
}

// LoadBlocks loads block from the reader.
func (bp *BLOCKPlugin) loadBlocks(sc *bufio.Reader, network []byte) (blocks []*BLOCK, err error) {
	for err == nil && sc.Size() > 80 {
		var b byte
		if b, err = sc.ReadByte(); err != nil {
			log.Println("failed to read byte", err.Error())
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

		// Current position
		var wireBlock *wire.MsgBlock
		var bytesNotRead int // block size
		var err2 error
		if wireBlock, bytesNotRead, err2 = bp.readBlock(sc); err2 != nil {
			log.Println("failed to read block", err2.Error())
			// Skip bytes
			if bytesNotRead > 0 {
				_, _ = sc.Discard(bytesNotRead)
			}
			continue
		}

		block := newBlocknetBlock(wireBlock)
		blocks = append(blocks, block)
	}

	if err == io.EOF { // not fatal
		err = nil
	}

	return
}

// readBlock deserializes bytes into block data.
func (bp *BLOCKPlugin) readBlock(sc *bufio.Reader) (block *wire.MsgBlock, size int, err error) {
	// read the block size
	sizeBytes := make([]byte, 4)
	if _, err = io.ReadFull(sc, sizeBytes); err != nil {
		log.Println("failed to read block size", err.Error())
		size = len(sizeBytes)
		return
	}
	size = int(binary.LittleEndian.Uint32(sizeBytes))

	// buffer for block bytes
	blockBytes := make([]byte, size)
	if _, err = io.ReadFull(sc, blockBytes); err != nil {
		log.Println("failed to read block bytes", err.Error())
		return
	}
	size = 0 // at this point all block bytes were read

	buf := bytes.NewReader(blockBytes)
	var header *wire.BlockHeader
	if header, err = bp.readBlockHeader(buf); err != nil {
		log.Println("failed to read block header", err.Error())
		return
	}
	block = wire.NewMsgBlock(header)

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
				if vins, vinLen, err = readVins(buf); err != nil {
					log.Println("failed to read tx vins 2", err.Error())
					return
				}
				if vouts, _, err = readVouts(buf); err != nil {
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
				if vins, _, err = readVins(buf); err != nil {
					log.Println("failed to read tx vins 2", err.Error())
					return
				}
				if vouts, _, err = readVouts(buf); err != nil {
					log.Println("failed to read tx vouts 2", err.Error())
					return
				}
			}
		} else { // no witness
			if vins, _, err = readVins(buf); err != nil {
				log.Println("failed to read tx vins 2", err.Error())
				return
			}
			if vouts, _, err = readVouts(buf); err != nil {
				log.Println("failed to read tx vouts 2", err.Error())
				return
			}
		}

		// TODO Handle witness marker flags

		// Locktime
		txLockTimeB := make([]byte, 4)
		if _, err = io.ReadFull(buf, txLockTimeB); err != nil {
			log.Println("failed to read tx locktime", err.Error())
			return
		}
		txLockTime := binary.LittleEndian.Uint32(txLockTimeB)

		tx := wire.NewMsgTx(int32(txVersion))
		tx.TxIn = vins
		tx.TxOut = vouts
		tx.LockTime = txLockTime
		block.Transactions = append(block.Transactions, tx)
	}

	return
}

// readBlockHeader deserializes block header.
func (bp *BLOCKPlugin) readBlockHeader(buf *bytes.Reader) (header *wire.BlockHeader, err error) {
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

// NewBLOCKPlugin returns new BLOCK plugin instance.
func NewBLOCKPlugin(blocksDir string) BlockLoader {
	plugin := &BLOCKPlugin{
		blocksDir: blocksDir,
		isReady: false,
		network: wire.MainNet,
		blocks: []*BLOCK{},
		txCache: make(map[string]map[string]*Tx),
		txIndex: make(map[wire.OutPoint]*BlockTx),
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
