package ws

import "strconv"

// Subject naming convention shared by the WS server, chat handlers, and
// the hub itself — centralized here so a typo can't silently create two
// different channels for what should be the same audience.
func AgentSubject(userID int64) string {
	return "agent-" + strconv.FormatInt(userID, 10)
}

func VisitorSubject(visitorID int64) string {
	return "visitor-" + strconv.FormatInt(visitorID, 10)
}

// DashboardSubject is per-merchant: every online Agent/Admin/Super Admin
// who can see that merchant subscribes to it, and receives new-chat,
// chat-updated, and presence-changed nudges so their Chat List / Overview
// dashboard can refresh without polling.
func DashboardSubject(merchantID int64) string {
	return "dashboard-" + strconv.FormatInt(merchantID, 10)
}
