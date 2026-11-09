package mobile

import (
    "strings"

    "github.com/crakacr-alt/Chameleon-Protocol/pkg/clientconfig"
)

// PrepareBridgeConfig validates a SOCKS-only Bridge profile and performs the
// same authenticated TLS/TCP first-hop probe used by the Android VPN mode.
// Unlike VpnService, this does not take the operating system's VPN slot: an
// existing overlay VPN can route connections to its private server endpoint.
// The caller must start this via the local sidecar service, not VpnService.
func PrepareBridgeConfig(configJSON, endpointOverride string) string {
    cfg, err := clientconfig.ParseJSON([]byte(configJSON))
    if err != nil {
        return "ERROR: " + err.Error()
    }
    cfg, err = clientconfig.PrepareBridge(cfg, endpointOverride)
    if err != nil {
        return "ERROR: " + err.Error()
    }
    data, err := clientconfig.JSON(cfg)
    if err != nil {
        return "ERROR: " + err.Error()
    }
    result := PrepareVPNConfig(string(data))
    if strings.HasPrefix(result, "ERROR:") {
        return "ERROR: Bridge: " + strings.TrimPrefix(result, "ERROR: ")
    }
    return result
}
