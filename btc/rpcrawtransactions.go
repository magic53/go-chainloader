package btc

import (
	"github.com/blocknetdx/go-exrplugins/data"
	"github.com/btcsuite/btcd/wire"
)

// GetRawTransaction calls getrawtransaction on the specified rpc endpoint.
func (bp *Plugin) GetRawTransaction(txid string) (*wire.MsgTx, error) {
	return data.PluginRPCGetRawTransaction(bp, txid, bp.TokenConf())
}

// GetRawTransactions calls getrawtransaction on the specified rpc endpoint.
func (bp *Plugin) GetRawTransactions(txids []string) ([]*wire.MsgTx, error) {
	return data.PluginRPCGetRawTransactions(bp, txids, bp.TokenConf())
}
