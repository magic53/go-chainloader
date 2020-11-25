// Copyright (c) 2020 Michael Madgett
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.
package data

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"github.com/btcsuite/btcd/wire"
	"log"
)

type RPCRawTransactionsPlugin interface {
	TokenPlugin
	ReadTransactionPlugin
	GetRawTransaction(txid string) (*wire.MsgTx, error)
	GetRawTransactions(txids []string) ([]*wire.MsgTx, error)
}

type rawTransaction struct {
	RPCResult
	RawTransaction string `json:"result"`
}

// PluginRPCGetRawTransaction fetches the raw transaction hex and parses into wire tx.
func PluginRPCGetRawTransaction(plugin ReadTransactionPlugin, txid string, tokenCfg TokenConfig) (tx *wire.MsgTx, err error) {
	params := []interface{}{txid} // txid
	var b []byte
	b, err = RPCRequest("getrawtransaction", tokenCfg, params)
	if err != nil {
		log.Println("failed to make getrawtransaction client request")
		return
	}
	var res rawTransaction
	if err = json.Unmarshal(b, &res); err != nil {
		log.Println("failed to parse getrawtransaction client request")
		return
	}
	if len(res.RawTransaction) < 32 {
		log.Printf("failed to parse getrawtransaction hex result for %s\n", txid)
		return
	}
	b, err = hex.DecodeString(res.RawTransaction)
	if err != nil {
		return
	}
	r := bytes.NewReader(b)
	tx, err = plugin.ReadTransaction(r)
	return
}

// PluginRPCGetRawTransactions calls getrawtransaction on each tx on the specified rpc endpoint.
func PluginRPCGetRawTransactions(plugin ReadTransactionPlugin, txids []string, tokenCfg TokenConfig) (txs []*wire.MsgTx, err error) {
	if len(txids) == 0 { // no work required
		return
	}

	queue := make(chan int, 20) // max requests at a time
	done := make(chan *wire.MsgTx)  // done notification queue, informs blocking queue when to proceed
	for i, txid := range txids {
		go func(i int, txid string) {
			queue <- i // block when queue is full
			res, err2 := PluginRPCGetRawTransaction(plugin, txid, tokenCfg)
			if err2 != nil { // TODO Error bubble up on individual rawtx fetch?
				done <- nil
				return
			}
			done <- res
		}(i, txid)
	}

	// Counter to track how many txs are left to process
	count := len(txids)

out:
	for {
		select {
		case tx := <-done:
			if tx != nil {
				txs = append(txs, tx)
			}
			<-queue // free up item in queue
			count--
			if count <= 0 {
				break out
			}
		}
	}

	return
}