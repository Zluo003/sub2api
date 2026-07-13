package service

import "testing"

func TestOrdinaryAPIKeysCannotBindSystemAgentGroup(t *testing.T) {
	service := &APIKeyService{}
	user := &User{}
	agent := &Group{ID: 9, Kind: "agent", SystemCode: "yingzo", IsExclusive: false}
	if service.canUserBindGroupInternal(user, agent, map[int64]bool{9: true}) {
		t.Fatal("system Agent group must be hidden from ordinary API Key binding")
	}
}
