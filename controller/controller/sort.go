package controller

import extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"

type sortByName []*extensions.Deployment

func (s sortByName) Len() int {
	return len(s)
}

func (s sortByName) Less(i, j int) bool {
	return s[i].GetName() < s[j].GetName()
}

func (s sortByName) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
