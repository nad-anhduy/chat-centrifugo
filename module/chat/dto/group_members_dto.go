package dto

// GroupMemberInfo is the payload returned by GET /api/v1/groups/:id/members.
type GroupMemberInfo struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar,omitempty"`
	Role     string `json:"role"` // "owner" | "member"
}
