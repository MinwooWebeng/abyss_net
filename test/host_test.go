package test

import (
	"context"
	"testing"

	abyss_host "abyss_neighbor_discovery/host"
	abyss_net "abyss_neighbor_discovery/net_service"
)

func TestHost(t *testing.T) {
	local_identity := abyss_net.NewBetaLocalIdentity("mallang")
	address_selector := abyss_net.NewBetaAddressSelector()
	netserv, _ := abyss_net.NewBetaNetService(local_identity, address_selector)
	abyss_host.NewAbyssNetHost(context.TODO(), netserv, nil)
}
