package btc

import (
	"github.com/btcsuite/btcd/chaincfg"
)

// MainNetParams returns the chain configuration for mainnet.
var MainNetParams = chaincfg.Params{
	Name:        "mainnet",
	Net:         0xd9b4bef9,
	DefaultPort: "8333",

	// Human-readable part for Bech32 encoded segwit addresses, as defined in
	// BIP 173.
	Bech32HRPSegwit: "bc",

	// Address encoding magics
	PubKeyHashAddrID:        0x00,
	ScriptHashAddrID:        0x05,
	PrivateKeyID:            0x80,
	WitnessPubKeyHashAddrID: 0x06, // starts with p2
	WitnessScriptHashAddrID: 0x0A, // starts with 7Xh

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x88, 0xad, 0xe4},
	HDPublicKeyID:  [4]byte{0x04, 0x88, 0xb2, 0x1e},

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 0x0,
}

// TestnetParams returns the chain configuration for testnet.
var TestnetParams = chaincfg.Params{
	Name:        "testnet",
	Net:         0x0709110b,
	DefaultPort: "18333",

	// Human-readable part for Bech32 encoded segwit addresses, as defined in
	// BIP 173.
	Bech32HRPSegwit: "tb",

	// Address encoding magics
	PubKeyHashAddrID:        0x6f,
	ScriptHashAddrID:        0xc4,
	PrivateKeyID:            0xef,
	WitnessPubKeyHashAddrID: 0x06, // starts with p2
	WitnessScriptHashAddrID: 0x0A, // starts with 7Xh

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x35, 0x83, 0x94},
	HDPublicKeyID:  [4]byte{0x04, 0x35, 0x87, 0xcf},

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 0x1,
}
