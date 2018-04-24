package controller

import apps "k8s.io/api/apps/v1"

type sortByName []*apps.Deployment

func (s sortByName) Len() int {
	return len(s)
}

func (s sortByName) Less(i, j int) bool {
	return s[i].GetName() < s[j].GetName()
}

func (s sortByName) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
