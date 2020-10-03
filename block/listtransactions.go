package block

import (
	"github.com/blocknetdx/go-exrplugins/data"
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
