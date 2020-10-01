package listtransactions

import (
	"encoding/binary"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"io"
	"log"
	"math"
	"os"
)

type BlockInterface interface {
	Block() *wire.MsgBlock
	Hash() chainhash.Hash
	Height() int64
	setHash(hash chainhash.Hash)
}

type BlockLoader interface {
	BlocksDir() string
	Ready() bool
	ClearIndex()
	LoadBlocks(blocksDir string, cfg *chaincfg.Params) error
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

// LoadPlugin opens the block database and loads block data.
func LoadPlugin(plugin BlockLoader) (err error) {
	log.Printf("Loading block database from '%s'", plugin.BlocksDir())
	if err = plugin.LoadBlocks(plugin.BlocksDir(), &chaincfg.MainNetParams); err != nil {
		return
	}
	return nil
}

// networkLE returns the little endian byte representation of the network magic number.
func networkLE(net wire.BitcoinNet) []byte {
	network := make([]byte, 4)
	binary.LittleEndian.PutUint32(network, uint32(net))
	return network
}

// readVins deserializes tx vins.
func readVins(buf io.ReadSeeker) (vins []*wire.TxIn, txVinsLen uint64, err error) {
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

// readVouts deserializes tx vouts.
func readVouts(buf io.ReadSeeker) (vouts []*wire.TxOut, txVoutLen uint64, err error) {
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

// shardsIter returns the iteration details for the current shard.
func shardsIter(shards int, currentShard int, rng int, remainder int) (start, end int) {
	start = currentShard * rng
	end = start + rng
	if currentShard == shards-1 {
		end += remainder // add remainder to last core
	}
	return
}

// shardsData returns an optimal shard count, range, and remainder for the
// desired shard count.
func shardsData(desiredShards int, blocksLen int) (shards, rng, remainder int) {
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

// fileExists returns whether the given file or directory exists.
func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
