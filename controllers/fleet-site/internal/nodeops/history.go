package nodeops

const (
	// MaxHistory is the maximum number of completed node operations retained on FleetClusterJoin status.
	MaxHistory = 20
	// MaxLogTailBytes is the amount of Ansible job log retained in history when the Job is gone.
	MaxLogTailBytes = 65536
)
