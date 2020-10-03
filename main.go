package main

import (
	"fmt"
	"github.com/blocknetdx/go-exrplugins/block"
	"github.com/blocknetdx/go-exrplugins/btc"
	"github.com/blocknetdx/go-exrplugins/data"
	"github.com/blocknetdx/go-exrplugins/ltc"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"
)

func init() {
	//log.SetFlags(0) // logging
	//log.SetOutput(ioutil.Discard)
}

func main() {
	var err error

	// Trace
	//f, err := os.Create("trace.out")
	//if err != nil {
	//	panic(err)
	//}
	//defer f.Close()
	//err = trace.Start(f)
	//if err != nil {
	//	panic(err)
	//}
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

	shutdown := make(chan os.Signal, 1)
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
	out:
		for {
			select {
			case sig := <-c:
				if sig == os.Interrupt {
					log.Printf("Shutting down: received signal %v\n", sig)
					data.ShutdownNow()
					shutdown <- sig
					break out
				}
			}
		}
	}()

	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
	}
	if !data.LoadTokenConfigs(dir) {
		log.Println("failed to load token configuration file: real time updates will be disabled")
	}

	var plugins []interface{}
	tokens := data.GetTokenConfigs()
	for _, config := range tokens {
		switch config.Ticker {
		case "BLOCK":
			// load block config
			plugin := block.NewPlugin(&block.MainNetParams, config)
			if err = plugin.LoadBlocks(plugin.BlocksDir()); err != nil {
				log.Println("BLOCK failed!", err.Error())
				return
			}
			plugins = append(plugins, plugin)
			//debugBLOCK(plugin, &config)
		case "LTC":
			// load ltc config
			plugin := ltc.NewPlugin(&ltc.MainNetParams, config)
			if err = plugin.LoadBlocks(plugin.BlocksDir()); err != nil {
				log.Println("LTC failed!", err.Error())
				return
			}
			plugins = append(plugins, plugin)
			//debugLTC(plugin, &config)
		case "BTC":
			// load btc config
			plugin := btc.NewPlugin(&btc.MainNetParams, config)
			if err = plugin.LoadBlocks(plugin.BlocksDir()); err != nil {
				log.Println("BTC failed!", err.Error())
				return
			}
			plugins = append(plugins, plugin)
			//debugBTC(plugin, &config)
		}
	}

	go watchMempools(plugins)

out:
	for {
		select {
		case sig := <-shutdown:
			if sig == os.Interrupt {
				log.Println("Exiting...")
				break out
			}
		}
	}
}

// watchMempools watches the mempool on a timer and saves new transaction data
// to disk.
func watchMempools(plugins []interface{}) {
	var counter uint64
	for {
		if data.IsShuttingDown() {
			break
		}
		if counter%30000 == 0 { // every ~30 seconds
			var wg sync.WaitGroup
			wg.Add(len(plugins))
			for _, plugin := range plugins {
				go func(plugin interface{}) {
					defer wg.Done()
					dataPlugin, ok := plugin.(data.Plugin)
					if !ok {
						return
					}
					mempoolPlugin, ok := plugin.(data.RPCMempoolPlugin)
					if !ok {
						return
					}
					rawTxPlugin, ok := plugin.(data.RPCRawTransactionsPlugin)
					if !ok {
						return
					}
					listTxPlugin, ok := plugin.(data.ListTransactionsPlugin)
					if !ok {
						return
					}
					mempool, err := mempoolPlugin.GetRawMempool()
					if err != nil {
						log.Printf("failed to getrawmempool on %s", dataPlugin.Ticker())
						return
					}
					if len(mempool) < 1 {
						return
					}
					wireTxs, err := rawTxPlugin.GetRawTransactions(mempool)
					if err != nil || len(wireTxs) < 1 {
						return
					}
					var txs []*data.Tx
					txs, err = dataPlugin.ImportTransactions(wireTxs)
					if err != nil {
						log.Printf("failed to import transactions for %s", dataPlugin.Ticker())
						return
					}
					fromMonth := time.Date(2020, 8, 1, 0, 0, 0, 0, time.UTC)
					toMonth := time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC)
					for _, tx := range txs {
						_ = listTxPlugin.WriteListTransactionsForAddress(tx.Address, fromMonth, toMonth, dataPlugin.TokenConf().ListTransactionsDir)
					}
				}(plugin)
			}
			wg.Wait()
		}
		time.Sleep(250 * time.Millisecond)
		counter += 250
	}
}

func debugBLOCK(blockPlugin *block.Plugin, config *data.TokenConfig) {
	var err error
	//var txids []string
	//txids, err = data.RPCRawMempool(config)
	//if err != nil {
	//	fmt.Println(err.Error())
	//}
	//txids = []string{
	//	"6fa76deb0382c1cee0c56f7f5e5ea7266c22bd6134d4738be2c1ec35ff8cf550",
	//	"6aaa8db67a20abfa86998c40a67bed9eb2ac01d57048488cc4db3a4e1f544bc9",
	//	"ea1d76239e42745a4ffcfe16b10060f01ac0441545da8bd052b73c1ce74b9135",
	//	"3d910d1265c33b023d520c99944e6c1987cc149a543096862a9299f142932b1b",
	//	"2f58de4b43538e794c862e9d62f8c9e559e3d4c62b5d0f4014ede11bb415eea7",
	//	"e402a318b8b3a95a499812220eb56ed07c0a35c2e61a9cdc9ecf0ec940abc34c",
	//	"1af5413887c3124b87f0e801c9d96bf86fa347b0f6febd7303360cf70fc52bde",
	//	"672ad61d9da652ad688cec1f10ab11f018361fc79ed63781291ee3551c8766b6",
	//	"6ddc1222d9aa9b0768e333f0aa2e44dad00a41a78b31f0268aa9cd85507e1857",
	//	"47534111299075b129305a74a914f56195ead27d3bff5e75d90f374603f2222b",
	//	"17e1f44ccda93cb8a198d5ab5179fb6f3fe8e20d521016b37146317d821ea829",
	//	"e09b625ec7b9e31f12987ff9541354f16e7ebfad4021114eb932b724a6478d8e",
	//	"8a8c227298ba90f4dc07a1d0efe73c1737292eaa83522b24ec41ce522f3a5290",
	//}
	//if rawtxs, err := data.RPCGetRawTransactions(blockPlugin, txids, config); err != nil {
	//	fmt.Println(err.Error())
	//} else {
	//	_, _ = blockPlugin.ImportTransactions(rawtxs)
	//}

	//txs, err := blockPlugin.ListTransactions(0, math.MaxInt32, []string{"BakbDabCMM1PVuFx8ruVM9AcWCWYfc66eV"})
	//txs, err := blockPlugin.ListTransactions(0, math.MaxInt32, []string{"BoWcezbZ9vFTwArtVTHJHp51zQZSGdcLXt"})
	//txs, err := blockPlugin.ListTransactions(0, math.MaxInt32, []string{"BreP7JHmYfp9YaGXBwN1F2X9BRq9sRdkiS"})
	//if err != nil {
	//	log.Println("BLOCK listtransactions failed!", err.Error())
	//	return
	//}
	//sort.Slice(txs, func(i, j int) bool {
	//	return txs[i].Time < txs[j].Time
	//})
	//if js, err2 := json.Marshal(txs); err2 == nil {
	//	fmt.Println(string(js))
	//}

	fromMonth := time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)
	toMonth := time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC)
	if err = blockPlugin.WriteListTransactions(fromMonth, toMonth, "/opt/blockchain/block/exrplugins/"); err != nil {
		fmt.Println("error", err.Error())
	}
}

func debugLTC(ltcPlugin *ltc.Plugin, config *data.TokenConfig) {
	var err error
	//var txids []string
	//txids, err = data.RPCRawMempool(config)
	//if err != nil {
	//	fmt.Println(err.Error())
	//}
	//// block 1922810
	//txids = []string{
	//	"645569e5ba9fd384dee3e19602cf5d4f81fc963c766def0f3fafd6afb5d6402b",
	//	"4e7178a1b6345230c6d47fa2b69a8300afd68668a94896606f066f697062d37b",
	//	"0afd7828844e12de18d4160281b27573bf54a676bf65bb8cfd2e79ccb474711b",
	//	"d19ed1aa14280693933a9c9e552ed7a294e537413e24780ddf9f865dde90546f",
	//	"eee223b6f6624ac079259fd379499513e6ac1440c68ddd1e20b91850fba66d22",
	//	"d2fad87261dcb381883e51953bbf7800ad02186180423a3138547296c165f0cd",
	//	"760f9f3385a0ada632c750f4603867504bdcda7be245b0b99de7bd065c27d5a3",
	//	"754421493b42ae1c2940b307d489b59c7bbdcb88d3535464b65519a997ca21a2",
	//	"e1ae5692f3b1189f4e38b9e0c8544d300fe83a4f1d40f524efec50e369b37901",
	//	"e0df0fd5f9b2c046ed754585ce45947753e79389272acff70c8cc9a094424db8",
	//	"8c7e73110c6a1d010d71221ffc35e32d14bd28d02ea8cec689f67e3a669045e6",
	//	"37294c514341cf787750d75d7ae4d4e7876cb355f8795f3ddd134df0f4910e07",
	//	"825de0fde9d159889dc3d2ac807cf39e6a4a3dfd7fbe9a3ebbc3d32271446eff",
	//	"49e64ab1747bffb08d6c61686588aa4bfe77a9b7015b4a45e03ed7385ad9d626",
	//	"fd1af7fed0f3aced908ec80355e5526152b05cad0973fff141ef7ad812b6a21c",
	//	"c549e50815c45e85092b2fa4c18706a54d2bcdfb515e1b921972e2e7d245542b",
	//	"0094b803cfac6f159d0cfe0cec80c7b658b28b8f41e71eb424b5380cb09475fc",
	//	"b0d59afccfd659b24bf72388263a71ea97e3c627171d32e31de35e95d14a0b27",
	//	"d62543ebe94d4de95bf037bd345782dd040f93fe95d7aa034661e5ca75c7e934",
	//	"4ee5dfd35dcf4920b5336f52cdd0402105e2099c5151f4ac932c62d91cf88037",
	//	"3061ab123835eb2fc68cd160c6c99e0f698512eec09e4c0b813a7af64d8f1b71",
	//	"1de4cf2323a6adda6a2616c5823bfffdea19731f29d252f691fe040a6f1cb877",
	//	"b540af13a49cad742614fa88cdfdf4ccfcdd4af9998a10106ee4b0ad471316e3",
	//	"0011e3e91dff7fca68a45386caf47e47bdc85fbc481ec694a5ab5c7b4e5cc19c",
	//	"eb9cda055b183fb063f5f9091d6c134268f029f72d022fe0fbf9997ef7426aa4",
	//	"9b46c4c4e65875780aebc31310621d96199ff3883a38eec08f1c9db5d2735aae",
	//	"e7d3d7636a28b7b2874e7edb8e2f4af859b9d58be6206f13b088cce4ac66e0b4",
	//	"e9bcafa9778ae022881fc42502b3e82a48664361472fad667b4e2686c0a8c6b5",
	//	"e7119ff43b250e3452f1329976c6aa8266ca6fc06c7df16959875e36fa44bfc6",
	//	"8d296fab683661b8b01552283d872bda80a9a43042172597f5618349eb6ecbc6",
	//	"50f8f6d82fac6c7f3b5a18a3a8e49d8211424b15e773556928b7872c76d5cdd6",
	//	"4b664a73e42cb6b72937abb6871c894a6a08957901f868534d56800dca8eefee",
	//	"907499cfd9752cd2e5caa2ce022f8942e7498be9d4abec429d4a6492f8a0a8f5",
	//}
	//if rawtxs, err := data.RPCGetRawTransactions(ltcPlugin, txids, config); err != nil {
	//	fmt.Println(err.Error())
	//} else {
	//	_, _ = ltcPlugin.ImportTransactions(rawtxs)
	//}
	//
	//txs, err := ltcPlugin.ListTransactions(0, math.MaxInt32, []string{"LV5nrreyVZJVvptA9PZSD4ViegKh7Qa8MA"})
	//if err != nil {
	//	log.Println("LTC listtransactions failed!", err.Error())
	//	return
	//}
	//sort.Slice(txs, func(i, j int) bool {
	//	return txs[i].Time < txs[j].Time
	//})
	//if js, err2 := json.Marshal(txs); err2 == nil {
	//	fmt.Println(string(js))
	//}

	fromMonth := time.Date(2020, 8, 1, 0, 0, 0, 0, time.UTC)
	toMonth := time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC)
	if err = ltcPlugin.WriteListTransactions(fromMonth, toMonth, "/opt/blockchain/block/exrplugins/"); err != nil {
		fmt.Println("error", err.Error())
	}
}

func debugBTC(btcPlugin *btc.Plugin, config *data.TokenConfig) {
	var err error
	//var txids []string
	//txids, err = data.RPCRawMempool(config)
	//if err != nil {
	//	fmt.Println(err.Error())
	//}
	//// block 1922810
	//txids = []string{
	//}
	//if rawtxs, err := data.RPCGetRawTransactions(ltcPlugin, txids, config); err != nil {
	//	fmt.Println(err.Error())
	//} else {
	//	_, _ = ltcPlugin.ImportTransactions(rawtxs)
	//}
	//
	//txs, err := btcPlugin.ListTransactions(0, math.MaxInt32, []string{"1F184JoctgpLnTQmABig3sJNG6QqkG9JuL"})
	//if err != nil {
	//	log.Println("BTC listtransactions failed!", err.Error())
	//	return
	//}
	//sort.Slice(txs, func(i, j int) bool {
	//	return txs[i].Time < txs[j].Time
	//})
	//if js, err2 := json.Marshal(txs); err2 == nil {
	//	fmt.Println(string(js))
	//}

	fromMonth := time.Date(2020, 8, 1, 0, 0, 0, 0, time.UTC)
	toMonth := time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC)
	if err = btcPlugin.WriteListTransactions(fromMonth, toMonth, "/opt/blockchain/block/exrplugins/"); err != nil {
		fmt.Println("error", err.Error())
	}
}
