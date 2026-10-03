package simplefin

import (
	"context"
	"errors"
	"testing"
)

// Go's IsPrivate/IsGlobalUnicast pair leaves several non-routable ranges
// open; the guard must refuse them too.
func TestResolveAllowed_RefusesReservedRanges(t *testing.T) {
	for _, host := range []string{
		"100.64.0.1",      // CGNAT (Tailscale, some cloud metadata)
		"100.100.100.200", // Alibaba Cloud metadata
		"192.0.0.9",       // IETF protocol assignments
		"198.18.0.1",      // benchmarking
		"240.0.0.1",       // reserved
		"64:ff9b::7f00:1", // NAT64 mapping of 127.0.0.1
		"64:ff9b:1::1",    // local-use NAT64
	} {
		if _, err := resolveAllowed(context.Background(), host); !errors.Is(err, errBlockedAddress) {
			t.Errorf("%s: err = %v, want errBlockedAddress", host, err)
		}
	}
}

func TestResolveAllowed_AcceptsPublicAddress(t *testing.T) {
	for _, host := range []string{"93.184.216.34", "2606:2800:220:1:248:1893:25c8:1946"} {
		if _, err := resolveAllowed(context.Background(), host); err != nil {
			t.Errorf("%s: unexpected err %v", host, err)
		}
	}
}
