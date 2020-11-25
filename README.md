# go-chainloader

This library will read raw chain dat files into memory and process transactions from raw blocks on disk. It stores all transactions for each address in a directory format suitable for being consumed by http clients or consumed by ETL processes. By accessing raw dat files service providers can leverage their existing datadirs instead of requesting data via the chain's rpc client which is significantly slower.

## Config

`config.yml` is required and tells the program where to find the .dat files for the respective chains.
Supported:
 - BTC
 - LTC
 - BLOCK
 
Sample `config.yml`:
```yaml
blockchain:
  block:
    ticker: BLOCK
    rpchost: localhost
    rpcport: 41414
    rpcuser: user
    rpcpass: pass
    segwitactivated: 1584537260
    blocksdir: "/opt/blockchain/block/blocks"
    txlimit: 1000
    outputdir: "/opt/blockchain/go-chainloader-data"
  ltc:
    ticker: LTC
    rpchost: localhost
    rpcport: 9332
    rpcuser: user
    rpcpass: pass
    segwitactivated: 1494432226
    blocksdir: "/opt/blockchain/ltc/blocks"
    txlimit: 100
    outputdir: "/opt/blockchain/go-chainloader-data"
  btc:
    ticker: BTC
    rpchost: localhost
    rpcport: 8332
    rpcuser: user
    rpcpass: pass
    segwitactivated: 1503539857
    blocksdir: "/opt/blockchain/btc/blocks"
    txlimit: 100
    outputdir: "/opt/blockchain/go-chainloader-data"

```

## Build

```
go get -d ./
go build main.go
./main
```