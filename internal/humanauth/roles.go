package humanauth

import (
	"encoding/json"
	"errors"
)

var (
	ErrInvalidRoleMapping = errors.New("invalid OIDC role mapping")
	ErrRoleDenied         = errors.New("OIDC role claim denied")
)

type RoleMapper struct {
	claim   string
	viewers map[string]struct{}
	admins  map[string]struct{}
}

func NewRoleMapper(claim string, viewerValues, adminValues []string) (RoleMapper, error) {
	if claim == "" || len(viewerValues) == 0 || len(adminValues) == 0 {
		return RoleMapper{}, ErrInvalidRoleMapping
	}
	mapper := RoleMapper{
		claim:   claim,
		viewers: make(map[string]struct{}, len(viewerValues)),
		admins:  make(map[string]struct{}, len(adminValues)),
	}
	for _, value := range viewerValues {
		if value == "" {
			return RoleMapper{}, ErrInvalidRoleMapping
		}
		mapper.viewers[value] = struct{}{}
	}
	for _, value := range adminValues {
		if value == "" {
			return RoleMapper{}, ErrInvalidRoleMapping
		}
		if _, overlaps := mapper.viewers[value]; overlaps {
			return RoleMapper{}, ErrInvalidRoleMapping
		}
		mapper.admins[value] = struct{}{}
	}
	return mapper, nil
}

func (mapper RoleMapper) Role(claims map[string]json.RawMessage) (Role, error) {
	raw, ok := claims[mapper.claim]
	if !ok {
		return "", ErrRoleDenied
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return mapper.match([]string{single})
	}
	var multiple []string
	if err := json.Unmarshal(raw, &multiple); err != nil || len(multiple) == 0 {
		return "", ErrRoleDenied
	}
	return mapper.match(multiple)
}

func (mapper RoleMapper) match(values []string) (Role, error) {
	for _, value := range values {
		if value == "" {
			return "", ErrRoleDenied
		}
		if _, ok := mapper.admins[value]; ok {
			return RoleAdmin, nil
		}
	}
	for _, value := range values {
		if _, ok := mapper.viewers[value]; ok {
			return RoleViewer, nil
		}
	}
	return "", ErrRoleDenied
}
