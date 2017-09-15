package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/caicloud/canary-release/pkg/version"
	proxyctl "github.com/caicloud/canary-release/proxies/nginx/controller"
	"github.com/caicloud/clientset/kubernetes"
	"github.com/caicloud/release-controller/pkg/kube"

	log "github.com/zoumo/logdog"
	"gopkg.in/urfave/cli.v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
)

// RunController start lb controller
func RunController(opts *Options) error {

	log.Notice("Controller Build Information", log.Fields{
		"release": version.RELEASE,
		"commit":  version.COMMIT,
		"repo":    version.REPO,
	})

	log.Info("Controller Running with", log.Fields{
		"debug":       opts.Debug,
		"kubconfig":   opts.Kubeconfig,
		"crname":      opts.Cfg.CanaryReleaseName,
		"crnamespace": opts.Cfg.CanaryReleaseNamespace,
	})

	if opts.Debug {
		log.ApplyOptions(log.DebugLevel)
	} else {
		log.ApplyOptions(log.InfoLevel)
	}

	// build config
	log.Infof("load kubeconfig from %s", opts.Kubeconfig)
	config, err := clientcmd.BuildConfigFromFlags("", opts.Kubeconfig)
	if err != nil {
		log.Fatal("Create kubeconfig error", log.Fields{"err": err})
		return err
	}

	// create clientset
	opts.Cfg.Client = kubernetes.NewForConfigOrDie(config)

	// create release client pool
	apiRes, err := kube.NewAPIResourcesByConfig(config)
	if err != nil {
		log.Fatalf("Errpr create api resources %v", err)
	}
	pool, err := kube.NewClientPool(scheme.Scheme, config, apiRes)
	if err != nil {
		log.Fatalf("Error create client pool for release %v", err)
		return err
	}
	codec := kube.NewYAMLCodec(scheme.Scheme, scheme.Scheme)
	client, err := kube.NewClient(pool, codec)
	if err != nil {
		log.Fatalf("Error create client for release %v", err)
		return err
	}
	opts.Cfg.Codec = codec
	opts.Cfg.ReleaseClient = client
	opts.Cfg.ReleaseClientPool = pool

	// start a controller on instances of lb
	controller := proxyctl.NewProxy(opts.Cfg)
	// handle shutdown
	go handleSigterm(controller)

	controller.Run(1)

	<-wait.NeverStop

	return nil
}

func main() {
	// fix for avoiding glog Noisy logs
	flag.CommandLine.Parse([]string{})

	app := cli.NewApp()
	app.Name = "canaryrelease-proxy"
	app.Version = version.RELEASE
	app.Compiled = time.Now()
	app.Usage = "k8s canaryrelease proxy"

	// add flags to app
	opts := NewOptions()
	opts.AddFlags(app)

	app.Action = func(c *cli.Context) error {
		if err := RunController(opts); err != nil {
			msg := fmt.Sprintf("running canaryrelease proxy failed, with err: %v\n", err)
			return cli.NewExitError(msg, 1)
		}
		return nil
	}

	// sort flags by name
	sort.Sort(cli.FlagsByName(app.Flags))

	app.Run(os.Args)

}

func handleSigterm(p *proxyctl.Proxy) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM)
	<-signalChan
	log.Infof("Received SIGTERM, shutting down")

	exitCode := 0
	if err := p.Stop(); err != nil {
		log.Infof("Error during shutdown %v", err)
		exitCode = 1
	}

	log.Infof("Exiting with %v", exitCode)
	os.Exit(exitCode)
}
