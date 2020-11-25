// Copyright (c) 2020 Michael Madgett
// Distributed under the MIT software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.
package block

import (
	"github.com/magic53/go-chainloader/data"
)

// GetRawMempool calls getrawmempool on the specified rpc endpoint.
func (bp *Plugin) GetRawMempool() ([]string, error) {
	return data.PluginGetRawMempool(bp)
}