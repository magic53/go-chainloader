// Copyright (c) 2020 Michael Madgett
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.
package data

import (
	"encoding/json"
	"errors"
)

type RPCMempoolPlugin interface {
	TokenPlugin
	GetRawMempool() ([]string, error)
}

type rawMempool struct {
	RPCResult
	Transactions []string `json:"result"`
}

// PluginGetRawMempool calls getrawmempool on the specified rpc endpoint.
func PluginGetRawMempool(bp RPCMempoolPlugin) ([]string, error) {
	b, err := RPCRequest("getrawmempool", bp.TokenConf(), nil)
	if err != nil {
		return nil, errors.New("failed to make getrawmempool client request")
	}
	var mempool rawMempool
	if err := json.Unmarshal(b, &mempool); err != nil {
		return nil, err
	}
	return mempool.Transactions, nil
}