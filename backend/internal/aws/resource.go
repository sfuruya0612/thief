package aws

// Resource is the common interface for all AWS resource types.
type Resource interface {
	ResourceID() string
	ResourceName() string
	// ResourceState returns a normalized state:
	// running | stopped | available | active | error | stale | updating | unknown
	ResourceState() string
	ServiceName() string
}

// NormalizeState maps service-specific state strings to normalized values.
func NormalizeState(raw string) string {
	switch raw {
	case "running", "RUNNING", "active", "ACTIVE", "available", "AVAILABLE",
		"enabled", "ENABLED", "deployed", "DEPLOYED":
		return "running"
	case "stopped", "STOPPED", "inactive", "INACTIVE", "disabled", "DISABLED":
		return "stopped"
	case "pending", "PENDING", "provisioning", "PROVISIONING",
		"creating", "CREATING", "starting", "STARTING":
		return "updating"
	case "error", "ERROR", "failed", "FAILED", "impaired", "IMPAIRED":
		return "error"
	case "updating", "UPDATING", "modifying", "MODIFYING", "rebooting", "REBOOTING":
		return "updating"
	default:
		return raw
	}
}
