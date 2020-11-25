// Copyright (c) 2020 Michael Madgett
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.
package block

import (
	"github.com/btcsuite/btcd/wire"
	"github.com/magic53/go-chainloader/data"
)

// GetRawTransaction calls getrawtransaction on the specified rpc endpoint.
func (bp *Plugin) GetRawTransaction(txid string) (*wire.MsgTx, error) {
	return data.PluginRPCGetRawTransaction(bp, txid, bp.TokenConf())
}

// GetRawTransactions calls getrawtransaction on the specified rpc endpoint.
func (bp *Plugin) GetRawTransactions(txids []string) ([]*wire.MsgTx, error) {
	return data.PluginRPCGetRawTransactions(bp, txids, bp.TokenConf())
}
