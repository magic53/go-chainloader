package main

import (
	"encoding/json"
	"fmt"
	"github.com/blocknetdx/go-exrplugins/block"
	"github.com/blocknetdx/go-exrplugins/data"
	"log"
	"math"
	"os"
	"path/filepath"
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

	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	if !data.LoadTokenConfigs(dir) {
		log.Println("failed to load token configuration file: real time updates will be disabled")
	}

	// load block blockConfig
	blockConfig, _ := data.TokenConfig("block")
	blockDir := "/opt/blockchain/block/data-bmainnet-goleveldb/blocks"
	blockPlugin := block.NewPlugin(&block.MainNetParams, blockDir, &blockConfig)
	if err = data.LoadBlocks(blockPlugin); err != nil {
		log.Println("BLOCK failed!", err.Error())
		return
	}

	// TODO Debug
	debug(blockPlugin, &blockConfig)

	// TODO ltcPlugin := listtransactions.NewLTCPlugin(ltcDir)
}

func debug(blockPlugin data.Plugin, config *data.Token) {
	var err error
	var txids []string
	txids, err = data.RPCRawMempool(config)
	if err != nil {
		fmt.Println(err.Error())
	}
	txids = []string{
		"6fa76deb0382c1cee0c56f7f5e5ea7266c22bd6134d4738be2c1ec35ff8cf550",
		"6aaa8db67a20abfa86998c40a67bed9eb2ac01d57048488cc4db3a4e1f544bc9",
		"ea1d76239e42745a4ffcfe16b10060f01ac0441545da8bd052b73c1ce74b9135",
		"3d910d1265c33b023d520c99944e6c1987cc149a543096862a9299f142932b1b",
		"2f58de4b43538e794c862e9d62f8c9e559e3d4c62b5d0f4014ede11bb415eea7",
		"e402a318b8b3a95a499812220eb56ed07c0a35c2e61a9cdc9ecf0ec940abc34c",
		"1af5413887c3124b87f0e801c9d96bf86fa347b0f6febd7303360cf70fc52bde",
		"672ad61d9da652ad688cec1f10ab11f018361fc79ed63781291ee3551c8766b6",
		"6ddc1222d9aa9b0768e333f0aa2e44dad00a41a78b31f0268aa9cd85507e1857",
		"47534111299075b129305a74a914f56195ead27d3bff5e75d90f374603f2222b",
		"17e1f44ccda93cb8a198d5ab5179fb6f3fe8e20d521016b37146317d821ea829",
		"e09b625ec7b9e31f12987ff9541354f16e7ebfad4021114eb932b724a6478d8e",
		"8a8c227298ba90f4dc07a1d0efe73c1737292eaa83522b24ec41ce522f3a5290",
	}
	if rawtxs, err := data.RPCGetRawTransactions(blockPlugin, txids, config); err != nil {
		fmt.Println(err.Error())
	} else {
		_, _ = blockPlugin.ImportTransactions(rawtxs)
	}

	//txs, err := blockPlugin.ListTransactions(0, math.MaxInt32, []string{"BakbDabCMM1PVuFx8ruVM9AcWCWYfc66eV"})
	//txs, err := blockPlugin.ListTransactions(0, math.MaxInt32, []string{"BoWcezbZ9vFTwArtVTHJHp51zQZSGdcLXt"})
	txs, err := blockPlugin.ListTransactions(0, math.MaxInt32, []string{"BreP7JHmYfp9YaGXBwN1F2X9BRq9sRdkiS"})
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
}