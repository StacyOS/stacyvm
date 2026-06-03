package ssh

import "strings"

// parseSandboxTarget extracts the target sandbox ID and an optional tenant hint
// from an SSH username. Supported forms: "<sandboxID>" and
// "<sandboxID>.<tenant>". Authorization is enforced against the connection's
// resolved identity by the Backend, so the tenant hint is advisory only.
func parseSandboxTarget(user string) (sandboxID, tenant string) {
	user = strings.TrimSpace(user)
	if user == "" {
		return "", ""
	}
	if i := strings.IndexByte(user, '.'); i >= 0 {
		return user[:i], user[i+1:]
	}
	return user, ""
}
