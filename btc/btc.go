package btc

import (
	"github.com/blocknetdx/go-exrplugins/data"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"io"
	"log"
	"time"
)

type Plugin struct {
	*data.ChainPlugin
}

// Ticker returns the ticker symbol (e.g. BLOCK, BTC, LTC).
func (bp *Plugin) Ticker() string {
	if bp.TokenCfg != nil && bp.TokenCfg.Ticker != "" {
		return bp.TokenCfg.Ticker
	} else {
		return "BTC"
	}
}

// SegwitActivated returns the segwit activation unix time.
func (bp *Plugin) SegwitActivated() int64 {
	if bp.TokenCfg != nil {
		return bp.TokenCfg.SegwitActivated
	} else {
		return 1503539857 // unix time
	}
}

// ReadBlock deserializes bytes into block
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
	return
}

// ReadTransaction deserializes bytes into transaction
func (bp *Plugin) ReadTransaction(buf io.ReadSeeker) (tx *wire.MsgTx, err error) {
	tx, err = data.ReadTransaction(buf, time.Now().Unix() >= bp.SegwitActivated())
	return
}

// NewPlugin returns new LTC plugin instance.
func NewPlugin(cfg *chaincfg.Params, blocksDir string, tokenCfg *data.Token) *Plugin {
	plugin := &Plugin{
		data.NewPlugin(cfg, blocksDir, tokenCfg),
	}
	plugin.BlockReader = plugin
	plugin.PluginOverrides = plugin
	return plugin
}
