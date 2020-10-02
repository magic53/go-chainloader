package data

import "fmt"

// Config describes the default configuration file format.
type Config struct {
	Blockchain map[string]Token `yaml:"blockchain"`
}

type Token struct {
	RPCHost         string `yaml:"rpchost"`
	RPCPort         int    `yaml:"rpcport"`
	RPCUser         string `yaml:"rpcuser"`
	RPCPass         string `yaml:"rpcpass"`
	SegwitActivated int64  `yaml:"segwitactivated"`
}

func (t *Token) RPCHttp() string {
	return fmt.Sprintf("http://%s:%v/", t.RPCHost, t.RPCPort)
}
