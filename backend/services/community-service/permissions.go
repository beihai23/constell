package main

// Permission bitmasks following the Discord-like permission model.
const (
	PermissionAdministrator  int64 = 1 << 0 // 1
	PermissionManageCommunity int64 = 1 << 1 // 2
	PermissionManageChannels int64 = 1 << 2 // 4
	PermissionManageRoles    int64 = 1 << 3 // 8
	PermissionKickMembers    int64 = 1 << 4 // 16
	PermissionBanMembers     int64 = 1 << 5 // 32
	PermissionManageMessages int64 = 1 << 6 // 64
	PermissionSendMessages   int64 = 1 << 7 // 128
	PermissionReadMessages   int64 = 1 << 8 // 256
	PermissionAttachFiles    int64 = 1 << 9 // 512
)

// allPermissions is the bitmask with every permission bit set.
const allPermissions = PermissionAdministrator | PermissionManageCommunity | PermissionManageChannels |
	PermissionManageRoles | PermissionKickMembers | PermissionBanMembers |
	PermissionManageMessages | PermissionSendMessages | PermissionReadMessages |
	PermissionAttachFiles

// ComputePermissions calculates the effective permission bitmask for a member.
func ComputePermissions(member *MemberRow, roles []*RoleRow, ownerID string) int64 {
	if member.UserID == ownerID {
		return allPermissions
	}

	var perms int64
	for _, role := range roles {
		perms |= role.Permissions
		if role.Permissions&PermissionAdministrator != 0 {
			return allPermissions
		}
	}
	return perms
}

// HasPermission checks if a computed permission bitmask includes the required permission.
func HasPermission(computed int64, required int64) bool {
	if computed&PermissionAdministrator != 0 {
		return true
	}
	return computed&required != 0
}
