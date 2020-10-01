package block

import (
	"github.com/blocknetdx/go-exrplugins/data"
)

func (bp *Plugin) ListTransactions(fromTime, toTime int64, addresses []string) (txs []*data.Tx, err error) {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	for _, address := range addresses {
		transactions, ok := bp.txCache[address]
		if !ok {
			continue
		}
		for _, tx := range transactions {
			if tx.Time >= fromTime && tx.Time <= toTime {
				txs = append(txs, tx)
			}
		}
	}
	return
}
