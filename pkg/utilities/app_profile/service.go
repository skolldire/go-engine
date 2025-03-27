package app_profile

import (
	"os"
	"strings"
)

func GetProfileByScope() string {
	tokens := strings.Split(GetScopeValue(), "-")
	if len(tokens) == 0 {
		return Local
	}
	return tokens[len(tokens)-1]
}

func IsLocalProfile() bool {
	return Local == GetScopeValue()
}

func IsTestProfile() bool {
	return strings.HasSuffix(GetScopeValue(), Test)
}

func IsProdProfile() bool {
	return strings.HasSuffix(GetScopeValue(), Prod)
}

func IsStageProfile() bool {
	return strings.HasSuffix(GetScopeValue(), Stage)
}

func GetScopeValue() string {
	scope := os.Getenv("SCOPE")
	if scope != "" {
		return scope
	}
	return Local
}
