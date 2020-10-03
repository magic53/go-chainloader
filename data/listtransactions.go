package data

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"
)

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

type writeListTxAddr struct {
	Address string
	TxMap   map[string]*Tx
}
type monthTxs struct {
	Address string
	Months []*monthTx
}
type monthTx struct {
	MonthDate time.Time
	TxsJson []byte
	Txs []*Tx
}
func (m *monthTx) Month() string {
	return m.MonthDate.Month().String()
}

// WriteListTransactions writes the listtransactions data to disk. This data is organized on disk by
// address and then month. The json file in this directory contains transaction output that is sorted
// ascending by time.
func (bp *ChainPlugin) WriteListTransactions(fromMonth time.Time, toMonth time.Time, txDir string) error {
	if exists, err := FileExists(txDir); !exists || err != nil {
		if err != nil {
			return errors.New(fmt.Sprintf("failed to write transactions to disk: %s", err.Error()))
		} else {
			return errors.New(fmt.Sprintf("failed to write transactions to disk, path does not exist: %s", txDir))
		}
	}

	// Read txs from cache, use provided read lock
	var txs []*writeListTxAddr
	bp.mu.RLock()
	for address, txmap := range bp.txCache {
		txs = append(txs, &writeListTxAddr{Address: address, TxMap: txmap})
	}
	bp.mu.RUnlock()

	txsLen := len(txs)
	shards, rng, remainder := ShardsData(runtime.NumCPU()*4, txsLen)

	var mu2 sync.Mutex
	var wg sync.WaitGroup
	wg.Add(shards)

	// Concurrently produce all the json bytes in memory required to write transactions to disk
	// TODO Limit memory requirements for WriteListTransactions
	for i := 0; i < shards; i++ {
		start, end := ShardsIter(shards, i, rng, remainder)
		go func() {
			fileDatas := make([]*monthTxs, end-start)
			for j := start; j < end; j++ {
				txData := txs[j]
				address := txData.Address
				txsMonths, err := listTxToJSON(txData.TxMap, fromMonth, toMonth)
				if err != nil {
					continue
				}
				mt := &monthTxs{Address: address, Months: txsMonths}
				fileDatas[j-start] = mt
			}
			if len(fileDatas) > 0 { // Write all files to disk
				mu2.Lock()
				for _, monthData := range fileDatas {
					for _, month := range monthData.Months {
						if len(month.Txs) < 1 {
							continue
						}
						file := fmt.Sprintf("%s.json", month.MonthDate.Format("2006-01"))
						_ = WriteListTransactionsForAddress(bp.Ticker(), monthData.Address, file, month.TxsJson, txDir)
					}
				}
				mu2.Unlock()
			}
			wg.Done()
		}()
	}
	wg.Wait()

	return nil
}

// WriteListTransactionsForAddress writes the transaction data to disk for the specified address.
func WriteListTransactionsForAddress(ticker string, address string, fileName string, jsonBytes []byte, txDir string) (err error) {
	path := ListTransactionsFile(ticker, address, fileName, txDir)
	var exists bool
	if exists, err = FileExists(path); err != nil {
		return
	}
	if !exists {
		if err = os.MkdirAll(filepath.Dir(path), 0775); err != nil {
			return
		}
	}
	if err = ioutil.WriteFile(path, jsonBytes, 0666); err != nil {
		log.Printf("failed to write transaction file for %s\n", address)
		return
	}
	return
}

// ListTransactionsFile returns the file on disk for the listtransactions address.
func ListTransactionsFile(ticker string, address string, fileName string, txDir string) string {
	monthDir := filepath.Join(txDir, "listtransactions", ticker, address, fileName)
	return monthDir
}

// listTxToJSON converts transaction data into json.
func listTxToJSON(txmap map[string]*Tx, fromMonth time.Time, toMonth time.Time) (txsMonths []*monthTx, err error) {
	firstMonth := time.Date(fromMonth.Year(), fromMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastMonth := time.Date(toMonth.Year(), toMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
	txsMonths = append(txsMonths, &monthTx{MonthDate: firstMonth})
	monthDate := firstMonth
	for monthDate.Unix() <= lastMonth.Unix() {
		monthDate = nextMonth(monthDate)
		txsMonths = append(txsMonths, &monthTx{MonthDate: monthDate})
	}

	// Add transactions into their respective month list
	for _, tx := range txmap {
		for _, month := range txsMonths {
			if tx.Time >= month.MonthDate.Unix() && tx.Time < nextMonth(month.MonthDate).Unix() {
				month.Txs = append(month.Txs, tx)
			}
		}
	}

	// Marshall each month txs into json array
	for _, month := range txsMonths {
		sort.Slice(month.Txs, func(i, j int) bool { // sort ascending by time
			return month.Txs[i].Time < month.Txs[j].Time
		})
		// Only include most recent 100 transactions in the month // TODO Review truncate
		sliced := month.Txs
		if len(sliced) > 100 {
			sliced = sliced[len(sliced)-100:]
		}
		var b []byte
		b, err = json.Marshal(sliced)
		if err != nil {
			log.Printf("failed to parse transaction data\n")
			continue
		}
		month.TxsJson = b
	}
	return
}

// nextMonth returns the month following the specified month.
func nextMonth(month time.Time) time.Time {
	nextMonth := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	nextMonth = month.AddDate(0, 2, -15) // ensure date normalizations don't impact calc
	nextMonth = time.Date(nextMonth.Year(), nextMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
	return nextMonth
}