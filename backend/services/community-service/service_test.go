package main

import (
	"testing"
)

func TestComputePermissionsOwner(t *testing.T) {
	member := &MemberRow{ServerID: "s1", UserID: "owner"}
	perms := ComputePermissions(member, nil, "owner")
	if perms&PermissionAdministrator == 0 {
		t.Fatal("owner should have ADMINISTRATOR")
	}
	if perms&PermissionSendMessages == 0 {
		t.Fatal("owner should have SEND_MESSAGES")
	}
	t.Log("owner has all permissions")
}

func TestComputePermissionsWithRoles(t *testing.T) {
	member := &MemberRow{ServerID: "s1", UserID: "user1"}
	roles := []*RoleRow{
		{ID: "r1", Permissions: PermissionKickMembers | PermissionBanMembers},
		{ID: "r2", Permissions: PermissionSendMessages | PermissionReadMessages},
	}
	perms := ComputePermissions(member, roles, "owner")

	if perms&PermissionKickMembers == 0 {
		t.Fatal("should have KICK_MEMBERS")
	}
	if perms&PermissionSendMessages == 0 {
		t.Fatal("should have SEND_MESSAGES")
	}
	if perms&PermissionAdministrator != 0 {
		t.Fatal("should NOT have ADMINISTRATOR")
	}
	t.Log("role-based permissions computed correctly")
}

func TestComputePermissionsAdministrator(t *testing.T) {
	member := &MemberRow{ServerID: "s1", UserID: "user1"}
	roles := []*RoleRow{{ID: "r1", Permissions: PermissionAdministrator}}
	perms := ComputePermissions(member, roles, "owner")

	if perms&PermissionManageServer == 0 {
		t.Fatal("ADMINISTRATOR should imply MANAGE_SERVER")
	}
	t.Log("ADMINISTRATOR grants all permissions")
}

func TestComputePermissionsNoRoles(t *testing.T) {
	member := &MemberRow{ServerID: "s1", UserID: "user1"}
	perms := ComputePermissions(member, nil, "owner")
	if perms != 0 {
		t.Fatalf("expected 0 permissions with no roles, got %d", perms)
	}
	t.Log("no roles = no permissions")
}

func TestHasPermission(t *testing.T) {
	tests := []struct {
		name     string
		computed int64
		required int64
		want     bool
	}{
		{"has", PermissionSendMessages, PermissionSendMessages, true},
		{"missing", PermissionReadMessages, PermissionSendMessages, false},
		{"admin_bypass", PermissionAdministrator, PermissionManageServer, true},
		{"zero", 0, PermissionReadMessages, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasPermission(tt.computed, tt.required); got != tt.want {
				t.Fatalf("HasPermission(%d, %d) = %v, want %v",
					tt.computed, tt.required, got, tt.want)
			}
		})
	}
}

func TestPermissionBitmaskValues(t *testing.T) {
	checks := []struct {
		name  string
		perm  int64
		value int64
	}{
		{"ADMINISTRATOR", PermissionAdministrator, 1},
		{"MANAGE_SERVER", PermissionManageServer, 2},
		{"MANAGE_CHANNELS", PermissionManageChannels, 4},
		{"MANAGE_ROLES", PermissionManageRoles, 8},
		{"KICK_MEMBERS", PermissionKickMembers, 16},
		{"BAN_MEMBERS", PermissionBanMembers, 32},
		{"MANAGE_MESSAGES", PermissionManageMessages, 64},
		{"SEND_MESSAGES", PermissionSendMessages, 128},
		{"READ_MESSAGES", PermissionReadMessages, 256},
		{"ATTACH_FILES", PermissionAttachFiles, 512},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if c.perm != c.value {
				t.Fatalf("expected %d, got %d", c.value, c.perm)
			}
		})
	}
}

func TestMarshalUnmarshalServer(t *testing.T) {
	s := &ServerRow{ID: "s1", Name: "Test", OwnerID: "u1"}
	data, err := MarshalServer(s)
	if err != nil {
		t.Fatalf("MarshalServer: %v", err)
	}
	got, err := UnmarshalServer(data)
	if err != nil {
		t.Fatalf("UnmarshalServer: %v", err)
	}
	if got.ID != s.ID || got.Name != s.Name {
		t.Fatalf("round-trip failed: %+v", got)
	}
	t.Log("Server marshal/unmarshal working")
}

func TestMarshalUnmarshalMembers(t *testing.T) {
	members := []*MemberRow{
		{ServerID: "s1", UserID: "u1"},
		{ServerID: "s1", UserID: "u2"},
	}
	data, err := MarshalMembers(members)
	if err != nil {
		t.Fatalf("MarshalMembers: %v", err)
	}
	got, err := UnmarshalMembers(data)
	if err != nil {
		t.Fatalf("UnmarshalMembers: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	t.Log("Members marshal/unmarshal working")
}

func TestMarshalUnmarshalRoles(t *testing.T) {
	roles := []*RoleRow{
		{ID: "r1", Permissions: PermissionAdministrator},
		{ID: "r2", Permissions: PermissionSendMessages},
	}
	data, err := MarshalRoles(roles)
	if err != nil {
		t.Fatalf("MarshalRoles: %v", err)
	}
	got, err := UnmarshalRoles(data)
	if err != nil {
		t.Fatalf("UnmarshalRoles: %v", err)
	}
	if len(got) != 2 || got[0].Permissions != PermissionAdministrator {
		t.Fatalf("round-trip failed")
	}
	t.Log("Roles marshal/unmarshal working")
}
