package config

import (
	"github.com/caicloud/clientset/kubernetes"

	cli "gopkg.in/urfave/cli.v1"
)

const (
	defaultProxy = "cargo.caicloudprivatetest.com/caicloud/canary-proxy-nginx:v0.1.0"
)

type Configuration struct {
	Client kubernetes.Interface
	Proxy  Proxy
}

type Proxy struct {
	Image string
}

// AddFlags add flags to app
func (c *Configuration) AddFlags(app *cli.App) {

	flags := []cli.Flag{
		cli.StringFlag{
			Name:        "proxy-image",
			Usage:       "`Image` of proxy",
			EnvVar:      "PROXY_IMAGE",
			Value:       defaultProxy,
			Destination: &c.Proxy.Image,
		},
	}

	app.Flags = append(app.Flags, flags...)

}
