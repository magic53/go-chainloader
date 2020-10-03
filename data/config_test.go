package data

import (
	"gopkg.in/yaml.v2"
	"testing"
)

func TestConfig(t *testing.T) {
	yml := []byte(
		"blockchain:" +
		"\n  block:" +
		"\n    ticker: BLOCK" +
		"\n    rpchost: localhost" +
		"\n    rpcport: 41414" +
		"\n    rpcuser: test" +
		"\n    rpcpass: pass" +
		"\n    segwitactivated: 12345" +
		"\n    blocksdir: /opt/blockchain/block" +
		"\n    txlimit: 1000" +
		"\n    listtransactionsdir: /opt/blockchain/block/listtransactions" +
		"\n  btc:" +
		"\n    ticker: BTC" +
		"\n    rpchost: localhost" +
		"\n    rpcport: 8332" +
		"\n    rpcuser: test1" +
		"\n    rpcpass: pass1" +
		"\n    segwitactivated: 123456" +
		"\n    blocksdir: /opt/blockchain/btc" +
		"\n    txlimit: 100" +
		"\n    listtransactionsdir: /opt/blockchain/btc/listtransactions" +
		"\n")
	var config Config
	if err := yaml.Unmarshal(yml, &config); err != nil {
		t.Errorf("failed to unmarshall yml %s", err.Error())
	}
	if config.Blockchain["block"].Ticker != "BLOCK" {
		t.Error("expecting BLOCK")
	}
	if config.Blockchain["block"].RPCHost != "localhost" {
		t.Error("expecting rpchost=localhost")
	}
	if config.Blockchain["block"].RPCPort != 41414 {
		t.Error("expecting rpcport=41414")
	}
	if config.Blockchain["block"].RPCUser != "test" {
		t.Error("expecting rpcuser=test")
	}
	if config.Blockchain["block"].RPCPass != "pass" {
		t.Error("expecting rpcpass=pass")
	}
	if config.Blockchain["block"].SegwitActivated != 12345 {
		t.Error("expecting 12345")
	}
	if config.Blockchain["block"].BlocksDir != "/opt/blockchain/block" {
		t.Error("expecting /opt/blockchain/block")
	}
	if config.Blockchain["block"].TxLimitPerMonth != 1000 {
		t.Error("expecting 1000")
	}
	if config.Blockchain["block"].ListTransactionsDir != "/opt/blockchain/block/listtransactions" {
		t.Error("expecting /opt/blockchain/block/listtransactions")
	}
	if config.Blockchain["block"].RPCHttp() != "http://localhost:41414/" {
		t.Error("expecting http://localhost:41414/")
	}
	if config.Blockchain["btc"].Ticker != "BTC" {
		t.Error("expecting BTC")
	}
	if config.Blockchain["btc"].RPCHost != "localhost" {
		t.Error("expecting rpchost=localhost")
	}
	if config.Blockchain["btc"].RPCPort != 8332 {
		t.Error("expecting rpcport=8332")
	}
	if config.Blockchain["btc"].RPCUser != "test1" {
		t.Error("expecting rpcuser=test1")
	}
	if config.Blockchain["btc"].RPCPass != "pass1" {
		t.Error("expecting rpcpass=pass1")
	}
	if config.Blockchain["btc"].SegwitActivated != 123456 {
		t.Error("expecting 123456")
	}
	if config.Blockchain["btc"].BlocksDir != "/opt/blockchain/btc" {
		t.Error("expecting /opt/blockchain/btc")
	}
	if config.Blockchain["btc"].TxLimitPerMonth != 100 {
		t.Error("expecting 100")
	}
	if config.Blockchain["btc"].ListTransactionsDir != "/opt/blockchain/btc/listtransactions" {
		t.Error("expecting /opt/blockchain/btc/listtransactions")
	}
	if config.Blockchain["btc"].RPCHttp() != "http://localhost:8332/" {
		t.Error("expecting http://localhost:8332/")
	}
}
