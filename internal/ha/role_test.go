package ha

import "testing"

func TestParseRole(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    Role
		wantErr bool
	}{
		{in: "", want: RoleLeader},
		{in: "leader", want: RoleLeader},
		{in: " Follower ", want: RoleFollower},
		{in: "FOLLOWER", want: RoleFollower},
		{in: "folower", wantErr: true},
		{in: "standby", wantErr: true},
	} {
		got, err := ParseRole(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseRole(%q) = %q, want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRole(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseRole(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRoleHelpers(t *testing.T) {
	if RoleLeader.IsFollower() || !RoleFollower.IsFollower() {
		t.Fatal("IsFollower is wrong")
	}
	if Role("").String() != "leader" {
		t.Fatalf("zero Role string = %q, want leader", Role("").String())
	}
	if RoleFollower.String() != "follower" {
		t.Fatalf("follower string = %q", RoleFollower.String())
	}
}
