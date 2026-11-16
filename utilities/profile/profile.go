package profile

import (
	"errors"
	"fmt"
	"strings"
)

type Profile string

const (
	Local Profile = "local"
	Test  Profile = "test"
	Stage Profile = "stage"
	Prod  Profile = "prod"
)

var ErrUnknownProfile = errors.New("unknown profile")

func ParseScope(scope string) (Profile, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return Local, nil
	}
	token := scope[strings.LastIndexByte(scope, '-')+1:]
	p := Profile(token)
	switch p {
	case Local, Test, Stage, Prod:
		return p, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownProfile, token)
	}
}
func (p Profile) String() string { return string(p) }
func (p Profile) IsLocal() bool  { return p == Local }
func (p Profile) IsTest() bool   { return p == Test }
func (p Profile) IsStage() bool  { return p == Stage }
func (p Profile) IsProd() bool   { return p == Prod }
