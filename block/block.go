package block

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

// SegwitActivated returns the segwit activation unix time.
func (bp *Plugin) SegwitActivated() int64 {
	if bp.TokenCfg != nil {
		return bp.TokenCfg.SegwitActivated
	} else {
		return 1584537260 // unix time
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

// ReadTransaction deserializes bytes into transaction
func (bp *Plugin) ReadTransaction(buf io.ReadSeeker) (tx *wire.MsgTx, err error) {
	tx, err = data.ReadTransaction(buf, time.Now().Unix() >= bp.SegwitActivated())
	return
}

// NewPlugin returns new BLOCK plugin instance.
func NewPlugin(cfg *chaincfg.Params, blocksDir string, tokenCfg *data.Token) data.Plugin {
	plugin := &Plugin{
		data.NewPlugin(cfg, blocksDir, tokenCfg),
	}
	plugin.BlockReader = plugin
	return plugin
}
