package main

import (
	"encoding/json"
	"fmt"
	"github.com/blocknetdx/go-exrplugins/block"
	"github.com/blocknetdx/go-exrplugins/data"
	"log"
	"math"
	"sort"
)

func init() {
	//log.SetFlags(0) // logging
	//log.SetOutput(ioutil.Discard)
}

func main() {
	// Trace
	//_ = trace.Start(os.Stdout)
	//defer trace.Stop()

	// Memory profiler
	//defer func() {
	//	f, err := os.Create("mem.prof")
	//	if err != nil {
	//		log.Fatal("could not create memory profile: ", err)
	//	}
	//	defer f.Close() // error handling omitted for example
	//	runtime.GC() // get up-to-date statistics
	//	if err := pprof.WriteHeapProfile(f); err != nil {
	//		log.Fatal("could not write memory profile: ", err)
	//	}
	//}()

	var err error
	blockDir := "/opt/blockchain/block/data-bmainnet-goleveldb/blocks"
	blockPlugin := block.NewPlugin(blockDir)
	if err = data.LoadPlugin(blockPlugin); err != nil {
		log.Println("BLOCK failed!", err.Error())
		return
	}

	txs, err := blockPlugin.ListTransactions(0, math.MaxInt32, []string{"BakbDabCMM1PVuFx8ruVM9AcWCWYfc66eV"})
	//txs, err := blockPlugin.ListTransactions(0, math.MaxInt32, []string{"BoWcezbZ9vFTwArtVTHJHp51zQZSGdcLXt"})
	if err != nil {
		log.Println("BLOCK listtransactions failed!", err.Error())
		return
	}
	sort.Slice(txs, func(i, j int) bool {
		return txs[i].Time < txs[j].Time
	})
	if js, err2 := json.Marshal(txs); err2 == nil {
		fmt.Println(string(js))
	}
	// TODO ltcPlugin := listtransactions.NewLTCPlugin(ltcDir)
}
