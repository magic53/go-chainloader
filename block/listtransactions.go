// Copyright (c) 2020 Michael Madgett
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.
package block

import (
	"github.com/magic53/go-chainloader/data"
	"time"
)

// ListTransactions lists all transactions during the time period.
func (bp *Plugin) ListTransactions(fromTime, toTime int64, addresses []string) ([]*data.Tx, error) {
	return data.PluginListTransactions(bp, fromTime, toTime, addresses)
}

// WriteListTransactions writes transactions to disk at the specified location.
// Uses the format [txDir]/listtransactions/BLOCK/BoWcezbZ9vFTwArtVTHJHp51zQZSGdcLXt/2020-06.json
func (bp *Plugin) WriteListTransactions(fromMonth time.Time, toMonth time.Time, txDir string) error {
	return data.PluginWriteListTransactions(bp, fromMonth, toMonth, "", txDir)
}

// WriteListTransactionsForAddress writes transactions to disk at the specified location.
// Uses the format [txDir]/listtransactions/BLOCK/BoWcezbZ9vFTwArtVTHJHp51zQZSGdcLXt/2020-06.json
func (bp *Plugin) WriteListTransactionsForAddress(address string, fromMonth time.Time, toMonth time.Time, txDir string) error {
	return data.PluginWriteListTransactions(bp, fromMonth, toMonth, address, txDir)
}
