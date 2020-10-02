package data

import (
	"gopkg.in/yaml.v2"
	"testing"
)

func TestConfig(t *testing.T) {
	yml := []byte(
		  "blockchain:" +
		"\n  block:" +
		"\n    rpchost: localhost" +
		"\n    rpcport: 41414" +
		"\n    rpcuser: test" +
		"\n    rpcpass: pass" +
		"\n  btc:" +
		"\n    rpchost: localhost" +
		"\n    rpcport: 8332" +
		"\n    rpcuser: test1" +
		"\n    rpcpass: pass1" +
		"\n")
	var config Config
	if err := yaml.Unmarshal(yml, &config); err != nil {
		t.Errorf("failed to unmarshall yml %s", err.Error())
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
}
