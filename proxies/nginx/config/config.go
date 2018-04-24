package config

import (
	"github.com/caicloud/clientset/kubernetes"
	"github.com/caicloud/rudder/pkg/kube"
	"k8s.io/client-go/rest"

	cli "gopkg.in/urfave/cli.v1"
)

type Configuration struct {
	config                 *rest.Config
	Client                 kubernetes.Interface
	ReleaseClient          kube.Client
	ReleaseClientPool      kube.ClientPool
	Codec                  kube.Codec
	CanaryReleaseName      string
	CanaryReleaseNamespace string
	ReleaseName            string
}

// AddFlags add flags to app
func (c *Configuration) AddFlags(app *cli.App) {

	flags := []cli.Flag{
		cli.StringFlag{
			Name:        "canary-release-name",
			Usage:       "the name of canary release",
			EnvVar:      "CANARY_RELEASE_NAME",
			Destination: &c.CanaryReleaseName,
		},
		cli.StringFlag{
			Name:        "canary-release-namespace",
			Usage:       "the namespace of canary release",
			EnvVar:      "CANARY_RELEASE_NAMESPACE",
			Destination: &c.CanaryReleaseNamespace,
		},
		cli.StringFlag{
			Name:        "release-name",
			Usage:       "the name of release",
			EnvVar:      "RELEASE_NAME",
			Destination: &c.ReleaseName,
		},
	}

	app.Flags = append(app.Flags, flags...)

}
