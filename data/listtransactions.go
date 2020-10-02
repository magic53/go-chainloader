package data

// ListTransactions returns all transactions over the time period that are associated
// with the specified addresses.
func (bp *ChainPlugin) ListTransactions(fromTime, toTime int64, addresses []string) (txs []*Tx, err error) {
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
