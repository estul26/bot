// Package domain defines shared domain constants and types.
package domain

const (
	// RoleOwner represents the bot owner with the highest privileges.
	RoleOwner = "owner"
	// RoleAdmin represents elevated administrators below the owner.
	RoleAdmin = "admin"
	// RoleUser represents a standard user with no elevated privileges.
	RoleUser = "user"
)

const (
	// RolePriorityOwner is the highest privilege level.
	RolePriorityOwner = 3
	// RolePriorityAdmin is the middle privilege level.
	RolePriorityAdmin = 2
	// RolePriorityUser is the base privilege level.
	RolePriorityUser = 1
)

// RolePriority returns the priority for the provided role, defaulting to 0 for
// unknown roles.
func RolePriority(role string) int {
	switch role {
	case RoleOwner:
		return RolePriorityOwner
	case RoleAdmin:
		return RolePriorityAdmin
	case RoleUser:
		return RolePriorityUser
	default:
		return 0
	}
}
