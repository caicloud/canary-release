package api

import (
	"reflect"
)

// Equal tests for equality between two L4Service types
func (s L4Service) Equal(s2 L4Service) bool {
	if len(s.Endpoints) != len(s2.Endpoints) {
		return false
	}
	for _, s1e := range s.Endpoints {
		found := false
		for _, s2e := range s2.Endpoints {
			if reflect.DeepEqual(s1e, s2e) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	s.Endpoints = nil
	s2.Endpoints = nil

	return reflect.DeepEqual(s, s2)
}
