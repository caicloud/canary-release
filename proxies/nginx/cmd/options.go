package main

import (
	"github.com/caicloud/canary-release/proxies/nginx/config"
	log "github.com/zoumo/logdog"
	"gopkg.in/urfave/cli.v1"
)

// Options contains controller options
type Options struct {
	Kubeconfig string
	Debug      bool
	Cfg        config.Configuration
}

// NewOptions reutrns a new Options
func NewOptions() *Options {
	return &Options{}
}

// AddFlags add flags to app
func (opts *Options) AddFlags(app *cli.App) {
	opts.Cfg.AddFlags(app)

	flags := []cli.Flag{
		cli.StringFlag{
			Name:        "kubeconfig",
			Usage:       "Path to a kube config. Only required if out-of-cluster.",
			Destination: &opts.Kubeconfig,
		},
		cli.BoolFlag{
			Name:        "debug",
			Usage:       "Run with debug mode",
			Destination: &opts.Debug,
		},
		cli.BoolFlag{
			Name:        "log-force-color",
			Usage:       "Force log to output with colore",
			Destination: &log.ForceColor,
		},
	}

	app.Flags = append(app.Flags, flags...)

}
