package wg

const (
	// OverlayInterfaceName is the WireGuard netdev on FleetClusterJoin nodes.
	// Inventory hub/spokes keep wg0; greenfield clusters use a separate overlay.
	OverlayInterfaceName = "wg-umkim"
	OverlayConfigBasename = "wg-umkim.conf"
)
