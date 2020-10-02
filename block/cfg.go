package block

import (
	"github.com/btcsuite/btcd/chaincfg"
)

// MainNetParams returns the chain configuration for mainnet.
var MainNetParams = chaincfg.Params{
	Name:        "mainnet",
	Net:         0xa3a2a0a1,
	DefaultPort: "41412",

	// Human-readable part for Bech32 encoded segwit addresses, as defined in
	// BIP 173.
	Bech32HRPSegwit: "block",

	// Address encoding magics
	PubKeyHashAddrID:        0x1a, // starts with B
	ScriptHashAddrID:        0x1c, // starts with C
	PrivateKeyID:            0x9a, // starts with 6 (uncompressed) or P (compressed)
	WitnessPubKeyHashAddrID: 0x06, // starts with p2
	WitnessScriptHashAddrID: 0x0A, // starts with 7Xh

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x88, 0xad, 0xe4}, // starts with xprv
	HDPublicKeyID:  [4]byte{0x04, 0x88, 0xb2, 0x1e}, // starts with xpub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 0x0,
}

// TestnetParams returns the chain configuration for testnet.
var TestnetParams = chaincfg.Params{
	Name:        "testnet",
	Net:         0xba657645,
	DefaultPort: "41474",

	// Human-readable part for Bech32 encoded segwit addresses, as defined in
	// BIP 173.
	Bech32HRPSegwit: "tblock",

	// Address encoding magics
	PubKeyHashAddrID:        0x8b, // starts with x or y
	ScriptHashAddrID:        0x13, // starts with 8
	PrivateKeyID:            0xef, // starts with 9 (uncompressed) or c (compressed)
	WitnessPubKeyHashAddrID: 0x06, // starts with p2
	WitnessScriptHashAddrID: 0x0A, // starts with 7Xh

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x3a, 0x80, 0x58, 0x37}, // starts with tprv
	HDPublicKeyID:  [4]byte{0x3a, 0x80, 0x61, 0xa0}, // starts with tpub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 0x1,
}

// RegressionNetParams returns the chain configuration for regtest.
var RegressionNetParams = chaincfg.Params{
	Name:        "regtest",
	Net:         0xac7ecfa1,
	DefaultPort: "41489",

	// Human-readable part for Bech32 encoded segwit addresses, as defined in
	// BIP 173.
	Bech32HRPSegwit: "blockrt",

	// Address encoding magics
	PubKeyHashAddrID:        0x8b, // starts with x or y
	ScriptHashAddrID:        0x13, // starts with 8
	PrivateKeyID:            0xef, // starts with 9 (uncompressed) or c (compressed)
	WitnessPubKeyHashAddrID: 0x06, // starts with p2
	WitnessScriptHashAddrID: 0x0A, // starts with 7Xh

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x3a, 0x80, 0x58, 0x37}, // starts with tprv
	HDPublicKeyID:  [4]byte{0x3a, 0x80, 0x61, 0xa0}, // starts with tpub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 0x1,
}
