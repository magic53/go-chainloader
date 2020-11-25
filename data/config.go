// Copyright (c) 2020 Michael Madgett
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.
package data

import "fmt"

// Config describes the default configuration file format.
type Config struct {
	Blockchain map[string]TokenConfig `yaml:"blockchain"`
}

type TokenConfig struct {
	Ticker          string `yaml:"ticker"`
	RPCHost         string `yaml:"rpchost"`
	RPCPort         int    `yaml:"rpcport"`
	RPCUser         string `yaml:"rpcuser"`
	RPCPass         string `yaml:"rpcpass"`
	SegwitActivated int64  `yaml:"segwitactivated"`
	BlocksDir       string `yaml:"blocksdir"`
	TxLimitPerMonth int    `yaml:"txlimit"`
	OutputDir       string `yaml:"outputdir"`
}

func (t TokenConfig) RPCHttp() string {
	return fmt.Sprintf("http://%s:%v/", t.RPCHost, t.RPCPort)
}

func (t TokenConfig) IsNull() bool {
	return t.Ticker == ""
}
