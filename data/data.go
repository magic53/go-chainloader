package data

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var shutdownInProgress int32 = 0
var registeredConfigs = map[string]*Token{}

type Block interface {
	Block() *wire.MsgBlock
	Hash() chainhash.Hash
	Height() int64
	setHash(hash chainhash.Hash)
}

type BlockReader interface {
	SegwitActivated() int64
	ReadBlock(buf io.ReadSeeker) (*wire.MsgBlock, error)
	ReadBlockHeader(buf io.ReadSeeker) (*wire.BlockHeader, error)
	ReadTransaction(buf io.ReadSeeker) (*wire.MsgTx, error)
}

type Plugin interface {
	BlockReader
	BlocksDir() string
	Ready() bool
	Network() wire.BitcoinNet
	Config() chaincfg.Params
	ClearIndex()
	LoadBlocks(blocksDir string) error
	AddBlocks(blocks []byte) ([]*Tx, error)
	ImportTransactions(transactions []*wire.MsgTx) ([]*Tx, error)
	ListTransactions(fromTime, toTime int64, addresses []string) ([]*Tx, error)
}

type Tx struct {
	Txid          string          `json:"txid"`
	Vout          int32           `json:"n"`
	Address       string          `json:"address"`
	Category      string          `json:"category"` // send, receive
	Amount        float64         `json:"amount"`
	Time          int64           `json:"time"`
	Confirmations uint32          `json:"confirmations"`
	Blockhash     *chainhash.Hash `json:"-"`
	OutP          *wire.OutPoint  `json:"-"`
}

func (tx *Tx) Key() string {
	return fmt.Sprintf("%v%v_%s", tx.Txid, tx.Vout, tx.Category)
}

func (tx *Tx) KeyCategory(txid string, vout int32, category string) string {
	return fmt.Sprintf("%v%v_%s", txid, vout, category)
}

type BlockTx struct {
	OutP        *wire.OutPoint
	Transaction *wire.MsgTx
}

// Shutdown starts the shutdown process. Thread safe.
func ShutdownNow() {
	atomic.StoreInt32(&shutdownInProgress, 1)
}

// IsShuttingDown returns true if shutdown was requested. Thread safe.
func IsShuttingDown() bool {
	return atomic.LoadInt32(&shutdownInProgress) == 1
}

// LoadTokenConfigs loads the token configurations at the specified path.
func LoadTokenConfigs(path string) bool {
	if filepath.Base(path) != "config.yml" && filepath.Base(path) != "config.yaml" {
		path2 := filepath.Join(path, "config.yml")
		if ok, err := FileExists(path2); !ok || err != nil {
			path2 = filepath.Join(path, "config.yaml")
			if ok, err := FileExists(path); !ok || err != nil {
				return false
			}
		}
		path = path2
	} else if ok, err := FileExists(path); !ok || err != nil {
		return false
	}
	b, err := ioutil.ReadFile(path)
	if err != nil {
		log.Printf("failed to read config file: %s\n", path)
		return false
	}
	var config Config
	if err := yaml.Unmarshal(b, &config); err != nil {
		log.Printf("failed to read config file, bad format: %s\n", err.Error())
		return false
	}
	for chain, tokencfg := range config.Blockchain {
		registeredConfigs[chain] = tokencfg
	}
	return true
}

// TokenConfig returns loaded token config with name.
func TokenConfig(name string) (Token, error) {
	if token, ok := registeredConfigs[name]; !ok {
		return Token{}, errors.New("No config found with name " + name)
	} else {
		return *token, nil
	}
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

// ExtractAddresses derives transactions from scriptPubKey.
func ExtractAddresses(scriptPk []byte, txHash string, txVout int, amount float64, blockTime time.Time, confirmations int,
	category string, blockHash *chainhash.Hash, outp *wire.OutPoint, cfg *chaincfg.Params) (txs []*Tx) {
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
			cacheTx := &Tx{
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

// LoadBlocks opens the block database and loads block data.
func LoadBlocks(plugin Plugin) (err error) {
	log.Printf("Loading block database from '%s'", plugin.BlocksDir())
	if err = plugin.LoadBlocks(plugin.BlocksDir()); err != nil {
		return
	}
	return nil
}

// BlockFiles finds suitable block files.
func BlockFiles(blocksDir string, re *regexp.Regexp, limit int) (files []os.FileInfo, err error) {
	exists := false
	if exists, err = FileExists(blocksDir); err != nil || !exists {
		if !exists {
			err = errors.New(fmt.Sprintf("File doesn't exist: %s", blocksDir))
		}
		return
	}

	var fileInfos []os.FileInfo
	if fileInfos, err = ioutil.ReadDir(blocksDir); err != nil {
		return
	}
	if len(fileInfos) == 0 { // check if no block files
		err = errors.New("BLOCK no db files found")
		return
	}

	// Filter out all non blk files
	for _, file := range fileInfos {
		if !re.MatchString(file.Name()) {
			continue
		}
		files = append(files, file)
	}

	// Sort most recent first
	sort.Slice(files, func(a, b int) bool {
		aName := files[a].Name()
		bName := files[b].Name()
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

	// Limit files returned
	dats := limit
	if dats <= len(files) {
		files = files[:dats]
	} else if dats < 0 {
		files = files[:1]
	}

	return
}

// NetworkLE returns the little endian byte representation of the network magic number.
func NetworkLE(net wire.BitcoinNet) []byte {
	network := make([]byte, 4)
	binary.LittleEndian.PutUint32(network, uint32(net))
	return network
}

// ReadVins deserializes tx vins.
func ReadVins(buf io.ReadSeeker) (vins []*wire.TxIn, txVinsLen uint64, err error) {
	if txVinsLen, err = wire.ReadVarInt(buf, 0); err != nil {
		log.Println("failed to read tx vin length", err.Error())
		return
	}
	for i := 0; i < int(txVinsLen); i++ {
		txHashB := make([]byte, 32)
		txNB := make([]byte, 4)
		if _, err = io.ReadFull(buf, txHashB); err != nil {
			log.Println("failed to read tx vin prevout hash", err.Error())
			return
		}
		if _, err = io.ReadFull(buf, txNB); err != nil {
			log.Println("failed to read tx vin prevout n", err.Error())
			return
		}

		// Outpoint
		var txHash *chainhash.Hash
		if txHash, err = chainhash.NewHash(txHashB); err != nil {
			return
		}
		txN := binary.LittleEndian.Uint32(txNB)
		outpoint := wire.NewOutPoint(txHash, txN)

		// ScriptSig
		var txScriptLen uint64
		txScriptLen, _ = wire.ReadVarInt(buf, 0) // non-fatal
		if txScriptLen > 1024 {
			err = errors.New("failed to read script sig, bad length")
			log.Println(err.Error())
			return
		}
		txScriptSigB := make([]byte, txScriptLen)
		_, _ = io.ReadFull(buf, txScriptSigB) // non-fatal

		// Tx sequence
		txSequenceB := make([]byte, 4)
		if _, err = io.ReadFull(buf, txSequenceB); err != nil {
			log.Println("failed to read tx vin sequence number", err.Error())
			return
		}
		txSequence := binary.LittleEndian.Uint32(txSequenceB)

		txIn := wire.NewTxIn(outpoint, txScriptSigB, nil)
		txIn.Sequence = txSequence
		vins = append(vins, txIn)
	}
	return
}

// ReadVouts deserializes tx vouts.
func ReadVouts(buf io.ReadSeeker) (vouts []*wire.TxOut, txVoutLen uint64, err error) {
	if txVoutLen, err = wire.ReadVarInt(buf, 0); err != nil {
		return
	}
	for i := 0; i < int(txVoutLen); i++ {
		// tx nValue
		txValueB := make([]byte, 8)
		if _, err = io.ReadFull(buf, txValueB); err != nil {
			return
		}
		txValue := int64(binary.LittleEndian.Uint64(txValueB))
		var txScriptPubKeyLen uint64
		if txScriptPubKeyLen, err = wire.ReadVarInt(buf, 0); err != nil {
			return
		}
		// tx script pubkey
		txScriptPubKeyB := make([]byte, txScriptPubKeyLen)
		if _, err = io.ReadFull(buf, txScriptPubKeyB); err != nil {
			return
		}
		vouts = append(vouts, wire.NewTxOut(txValue, txScriptPubKeyB))
	}
	return
}

// ReadBlock reads the block.
func ReadBlock(buf io.ReadSeeker, header *wire.BlockHeader, witnessTime int64) (block *wire.MsgBlock, err error) {
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
		var tx *wire.MsgTx
		if tx, err = ReadTransaction(buf, block.Header.Timestamp.Unix() >= witnessTime); err != nil {
			return
		}
		block.Transactions = append(block.Transactions, tx)
	}

	return
}

// ReadBlockHeader reads the block header into the default Bitcoin header.
func ReadBlockHeader(buf io.ReadSeeker) (header *wire.BlockHeader, err error) {
	versionB := make([]byte, 4)
	prevBlockB := make([]byte, 32)
	merkleB := make([]byte, 32)
	blockTimeB := make([]byte, 4)
	bitsB := make([]byte, 4)
	nonceB := make([]byte, 4)

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

// ReadTransaction reads the transaction.
func ReadTransaction(buf io.ReadSeeker, txAllowWitness bool) (tx *wire.MsgTx, err error) {
	var vins []*wire.TxIn
	var vouts []*wire.TxOut

	txVersionB := make([]byte, 4)
	if _, err = io.ReadFull(buf, txVersionB); err != nil {
		log.Println("failed to read tx version", err.Error())
		return
	}
	txVersion := binary.LittleEndian.Uint32(txVersionB)

	if txAllowWitness {
		txWitnessMarker := make([]byte, 2)
		if _, err = io.ReadFull(buf, txWitnessMarker); err != nil {
			log.Println("failed to read tx vins witness marker", err.Error())
			return
		}
		if bytes.Equal(txWitnessMarker, []byte{0x0, 0x1}) {
			var vinLen uint64
			if vins, vinLen, err = ReadVins(buf); err != nil {
				log.Println("failed to read tx vins 2", err.Error())
				return
			}
			if vouts, _, err = ReadVouts(buf); err != nil {
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
			if vins, _, err = ReadVins(buf); err != nil {
				log.Println("failed to read tx vins 2", err.Error())
				return
			}
			if vouts, _, err = ReadVouts(buf); err != nil {
				log.Println("failed to read tx vouts 2", err.Error())
				return
			}
		}
	} else { // no witness
		if vins, _, err = ReadVins(buf); err != nil {
			log.Println("failed to read tx vins 2", err.Error())
			return
		}
		if vouts, _, err = ReadVouts(buf); err != nil {
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

	tx = &wire.MsgTx{
		Version:  int32(txVersion),
		TxIn:     vins,
		TxOut:    vouts,
		LockTime: txLockTime,
	}
	return
}

// ShardsIter returns the iteration details for the current shard.
func ShardsIter(shards int, currentShard int, rng int, remainder int) (start, end int) {
	start = currentShard * rng
	end = start + rng
	if currentShard == shards-1 {
		end += remainder // add remainder to last core
	}
	return
}

// ShardsData returns an optimal shard count, range, and remainder for the
// desired shard count.
func ShardsData(desiredShards int, blocksLen int) (shards, rng, remainder int) {
	shards = desiredShards
	rng = blocksLen
	remainder = 0
	if shards > blocksLen {
		shards = blocksLen
	}
	if rng > shards {
		if rng%shards != 0 {
			remainder = rng % shards
		}
		rng = int(math.Floor(float64(rng) / float64(shards)))
	}
	return
}

// NextBlock finds all the blocks in the buffer and sends a seeked buffer
// to a delegate handler. This is a long running operation and checks for
// shutdown.
func NextBlock(sc *bufio.Reader, network []byte, handle func([]byte) bool) (bool, error) {
	var err error
	ok := false
	count := 0
	for err == nil && sc.Size() > 80 {
		var b byte
		if b, err = sc.ReadByte(); err != nil {
			if err != io.EOF {
				log.Println("failed to read byte", err.Error())
				ok = false
			} else {
				ok = true
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
				ok = false
				break
			}
			log.Println("failed to copy block bytes", err2.Error())
			if size-n > 0 { // Skip bytes
				_, _ = sc.Discard(size - n)
			}
			continue
		}

		if !handle(blockBytes) { // ask delegate if we can proceed
			ok = false
			break
		}

		count++
		if count % 10000 == 0 && IsShuttingDown() {
			break
		}
	}

	if err == nil {
		ok = true
	} else if err == io.EOF { // not fatal
		err = nil
		ok = true
	}

	return ok, err
}

type RPCResult struct {
	Error  interface{} `json:"error,omitempty"`
	Id     string      `json:"id,omitempty"`
}
type rawMempool struct {
	RPCResult
	Transactions []string `json:"result"`
}
type rawTransaction struct {
	RPCResult
	Transaction string `json:"result"`
}

// RPCRawMempool calls getrawtransaction on each tx on the specified rpc endpoint.
func RPCGetRawTransactions(plugin Plugin, txids []string, tokencfg *Token) ([]*wire.MsgTx, error) {
	if len(txids) == 0 { // no work required
		return nil, nil
	}

	queue := make(chan int, 20) // max requests at a time
	done := make(chan *string)   // done notification queue, informs blocking queue when to proceed
	for i, txid := range txids {
		params := make([]interface{}, 0)
		params = append(params, txid)
		go func(i int) {
			queue <- i // block when queue is full
			b, err := RPCRequest("getrawtransaction", tokencfg, params)
			if err != nil {
				log.Println("failed to make getrawmempool client request")
				done <- nil
				return
			}
			var res rawTransaction
			if err := json.Unmarshal(b, &res); err != nil {
				log.Println("failed to parse getrawmempool client request")
				done <- nil
				return
			}
			done <- &res.Transaction
		}(i)
	}
	count := len(txids)
	var rawtxs []string

out:
	for {
		select {
		case tx := <-done:
			if tx != nil {
				rawtxs = append(rawtxs, *tx)
			}
			<-queue // free up item in queue
			count--
			if count <= 0 {
				break out
			}
		}
	}

	// Deserialize raw txs
	var txs []*wire.MsgTx
	for _, rawtx := range rawtxs {
		b, err := hex.DecodeString(rawtx)
		if err != nil {
			continue
		}
		r := bytes.NewReader(b)
		if tx, err := plugin.ReadTransaction(r); err != nil {
			continue
		} else {
			txs = append(txs, tx)
		}
	}

	return txs, nil
}

// RPCRawMempool calls getrawmempool on the specified rpc endpoint.
func RPCRawMempool(tokencfg *Token) ([]string, error) {
	b, err := RPCRequest("getrawmempool", tokencfg, nil)
	if err != nil {
		return nil, errors.New("failed to make getrawmempool client request")
	}
	var mempool rawMempool
	if err := json.Unmarshal(b, &mempool); err != nil {
		return nil, err
	}
	return mempool.Transactions, nil
}

// RPCRequest performs suitable rpc client request with basic auth and timeout.
func RPCRequest(method string, tokencfg *Token, params []interface{}) ([]byte, error) {
	paramList := ""
	for _, val := range params {
		switch v := val.(type) {
		case int:
			paramList += fmt.Sprintf("%v", v)
		case []string:
			paramList += fmt.Sprintf("%s", strings.Join(v, ","))
		default:
			paramList += fmt.Sprintf("\"%s\"", v)
		}
	}
	vars := []byte(fmt.Sprintf(`{"jsonrpc":"1.0", "id":"go-exrplugins", "method":"%s", "params":[%s]}`, method, paramList))
	r := bytes.NewReader(vars)
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	req, err := http.NewRequest("POST", tokencfg.RPCHttp(), r)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(tokencfg.RPCUser, tokencfg.RPCPass)
	req.Header.Set("content-type", "text/plain")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return b, err
}

// FileExists returns whether the given file or directory exists.
func FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
