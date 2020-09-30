package listtransactions

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"io"
	"log"
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
	LoadBlocks(blocksDir string) error
	ListTransactions(fromTime, toTime int64, addresses []string) ([]*Tx, error)
}

type Tx struct {
	Txid          string         `json:"txid"`
	Vout          int32          `json:"n"`
	Address       string         `json:"address"`
	Category      string         `json:"category"` // send, receive
	Amount        float64        `json:"amount"`
	Time          int64          `json:"time"`
	Confirmations uint32         `json:"confirmations"`
	Blockhash     chainhash.Hash `json:"-"`
	OutP          wire.OutPoint  `json:"-"`
}

func (tx *Tx) Key() string {
	return fmt.Sprintf("%v%v_%s", tx.Txid, tx.Vout, tx.Category)
}

func (tx *Tx) KeyCategory(txid string, vout int32, category string) string {
	return fmt.Sprintf("%v%v_%s", txid, vout, category)
}

type BlockTx struct {
	Block       BlockInterface
	Transaction *wire.MsgTx
}

// LoadPlugin opens the block database and loads block data.
func LoadPlugin(plugin BlockLoader) (err error) {
	log.Printf("Loading block database from '%s'", plugin.BlocksDir())
	if err = plugin.LoadBlocks(plugin.BlocksDir()); err != nil {
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
func readVins(buf *bytes.Reader) (vins []*wire.TxIn, txVinsLen uint64, err error) {
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
func readVouts(buf *bytes.Reader) (vouts []*wire.TxOut, txVoutLen uint64, err error) {
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
