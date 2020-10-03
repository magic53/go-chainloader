package ltc

import (
	"github.com/blocknetdx/go-exrplugins/data"
)

// GetRawMempool calls getrawmempool on the specified rpc endpoint.
func (bp *Plugin) GetRawMempool() ([]string, error) {
	return data.PluginGetRawMempool(bp)
}