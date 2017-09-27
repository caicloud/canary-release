package api

import (
	"k8s.io/client-go/pkg/api/v1"
)

// L4Service describes a L4 service.
type L4Service struct {
	// Port extenrnal port to expose
	Port int32 `json:"port"`
	// Backend of the service
	Backend L4Backend `json:"backend"`
	// Endpoints active endpoints of the service
	Endpoints []Endpoint `json:"endpoints"`
}

// L4Backend describes the kubernetes service behind L4 Ingress service
type L4Backend struct {
	Port      int32       `json:"port"`
	Name      string      `json:"name"`
	Namespace string      `json:"namespace"`
	Protocol  v1.Protocol `json:"protocol"`
}

// Endpoint describes a endpoint in a backend
// In this case, it is also a kubernetes server not endpoirnt
type Endpoint struct {
	// Address IP address of the endpoint
	Address string `json:"address"`
	// Port number of the port
	Port int32 `json:"port"`
	// Weight of the endpoint
	Weight int32 `json:"weight"`
}
