package gencmd

import "github.com/trueforge-org/clustertool/pkg/talosconfig"

// Initial setup must not route through other control planes that are still in
// maintenance and do not yet have the cluster's certificates.
func bootstrapCommand(inv *talosconfig.Inventory, operation string, extra ...string) Command {
	node := inv.Bootstrap()
	return nodeCommand(operation, node, append([]string{"-e", node.Address}, extra...)...)
}

// An interrupted bootstrap RPC may already have succeeded. Preserve the etcd
// membership check before sending another bootstrap request when resuming.
func bootstrapIfNeeded(inv *talosconfig.Inventory) error {
	if _, err := ExistingControlPlane(inv); err == nil {
		return nil
	}
	return ExecCmd(bootstrapCommand(inv, "bootstrap"))
}
