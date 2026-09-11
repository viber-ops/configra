package humanauth_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/viber-ops/configra/internal/humanauth"
)

func TestRoleMapperMapsOnlyExplicitStringClaims(t *testing.T) {
	mapper, err := humanauth.NewRoleMapper("groups", []string{"developers"}, []string{"configra-admins"})
	if err != nil {
		t.Fatalf("NewRoleMapper: %v", err)
	}
	tests := []struct {
		name    string
		claim   string
		want    humanauth.Role
		wantErr error
	}{
		{name: "viewer array", claim: `["developers"]`, want: humanauth.RoleViewer},
		{name: "admin array", claim: `["developers","configra-admins"]`, want: humanauth.RoleAdmin},
		{name: "viewer string", claim: `"developers"`, want: humanauth.RoleViewer},
		{name: "missing", wantErr: humanauth.ErrRoleDenied},
		{name: "unmatched", claim: `["other"]`, wantErr: humanauth.ErrRoleDenied},
		{name: "wrong type", claim: `{"name":"developers"}`, wantErr: humanauth.ErrRoleDenied},
		{name: "empty value", claim: `""`, wantErr: humanauth.ErrRoleDenied},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := map[string]json.RawMessage{}
			if test.claim != "" {
				claims["groups"] = json.RawMessage(test.claim)
			}
			role, err := mapper.Role(claims)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Role error = %v, want %v", err, test.wantErr)
			}
			if role != test.want {
				t.Fatalf("Role = %q, want %q", role, test.want)
			}
		})
	}
}

func TestRoleMapperRejectsUnsafeConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		claim   string
		viewers []string
		admins  []string
	}{
		{name: "missing claim", viewers: []string{"developers"}, admins: []string{"admins"}},
		{name: "missing viewer values", claim: "groups", admins: []string{"admins"}},
		{name: "missing admin values", claim: "groups", viewers: []string{"developers"}},
		{name: "overlap", claim: "groups", viewers: []string{"shared"}, admins: []string{"shared"}},
		{name: "empty value", claim: "groups", viewers: []string{""}, admins: []string{"admins"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := humanauth.NewRoleMapper(test.claim, test.viewers, test.admins); !errors.Is(err, humanauth.ErrInvalidRoleMapping) {
				t.Fatalf("NewRoleMapper error = %v, want ErrInvalidRoleMapping", err)
			}
		})
	}
}
