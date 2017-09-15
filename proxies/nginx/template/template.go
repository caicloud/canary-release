package template

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	textTemplate "text/template"

	"github.com/caicloud/canary-release/proxies/nginx/config"
	"github.com/golang/glog"
	log "github.com/zoumo/logdog"
	"k8s.io/ingress/core/pkg/watch"

	ingressConfig "k8s.io/ingress/controllers/nginx/pkg/config"
)

const (
	slash         = "/"
	defBufferSize = 65535
)

var (
	funcMap = textTemplate.FuncMap{
		"empty": func(input interface{}) bool {
			check, ok := input.(string)
			if ok {
				return len(check) == 0
			}
			return true
		},
		// "buildLocation":            buildLocation,
		// "buildAuthLocation":        buildAuthLocation,
		// "buildAuthResponseHeaders": buildAuthResponseHeaders,
		// "buildProxyPass":           buildProxyPass,
		// "buildRateLimitZones":      buildRateLimitZones,
		// "buildRateLimit":           buildRateLimit,
		// "buildResolvers":           buildResolvers,
		// "buildUpstreamName":        buildUpstreamName,
		// "isLocationAllowed":        isLocationAllowed,
		// "buildDenyVariable":        buildDenyVariable,
		"buildLogFormatUpstream": buildLogFormatUpstream,
		"getenv":                 os.Getenv,
		"contains":               strings.Contains,
		"hasPrefix":              strings.HasPrefix,
		"hasSuffix":              strings.HasSuffix,
		"toUpper":                strings.ToUpper,
		"toLower":                strings.ToLower,
		"formatIP":               formatIP,
		"buildNextUpstream":      buildNextUpstream,
	}
)

// Template ...
type Template struct {
	tmpl      *textTemplate.Template
	fw        watch.FileWatcher
	s         int
	tmplBuf   *bytes.Buffer
	outCmdBuf *bytes.Buffer
}

//NewTemplate returns a new Template instance or an
//error if the specified template file contains errors
func NewTemplate(file string, onChange func()) (*Template, error) {
	tmpl, err := textTemplate.New("nginx.tmpl").Funcs(funcMap).ParseFiles(file)
	if err != nil {
		return nil, err
	}
	fw, err := watch.NewFileWatcher(file, onChange)
	if err != nil {
		return nil, err
	}

	return &Template{
		tmpl:      tmpl,
		fw:        fw,
		s:         defBufferSize,
		tmplBuf:   bytes.NewBuffer(make([]byte, 0, defBufferSize)),
		outCmdBuf: bytes.NewBuffer(make([]byte, 0, defBufferSize)),
	}, nil
}

// Close removes the file watcher
func (t *Template) Close() {
	t.fw.Close()
}

// Write populates a buffer using a template with NGINX configuration
// and the servers and upstreams created by Ingress rules
func (t *Template) Write(conf config.TemplateConfig) ([]byte, error) {
	defer t.tmplBuf.Reset()
	defer t.outCmdBuf.Reset()

	defer func() {
		if t.s < t.tmplBuf.Cap() {
			log.Debug("adjusting template buffer size from %v to %v", t.s, t.tmplBuf.Cap())
			t.s = t.tmplBuf.Cap()
			t.tmplBuf = bytes.NewBuffer(make([]byte, 0, t.tmplBuf.Cap()))
			t.outCmdBuf = bytes.NewBuffer(make([]byte, 0, t.outCmdBuf.Cap()))
		}
	}()

	log.Debug("NGINX configuration", log.Fields{"conf": conf})

	err := t.tmpl.Execute(t.tmplBuf, conf)
	if err != nil {
		return nil, err
	}

	// squeezes multiple adjacent empty lines to be single
	// spaced this is to avoid the use of regular expressions
	cmd := exec.Command("/controller/clean-nginx-conf.sh")
	cmd.Stdin = t.tmplBuf
	cmd.Stdout = t.outCmdBuf
	if err := cmd.Run(); err != nil {
		log.Warningf("unexpected error cleaning template: %v", err)
		return t.tmplBuf.Bytes(), nil
	}

	return t.outCmdBuf.Bytes(), nil
}

// fomatIP will wrap IPv6 addresses in [] and return IPv4 addresses
// without modification. If the input cannot be parsed as an IP address
// it is returned without modification.
func formatIP(input string) string {
	ip := net.ParseIP(input)
	if ip == nil {
		return input
	}
	if v4 := ip.To4(); v4 != nil {
		return input
	}
	return fmt.Sprintf("[%s]", input)
}

func buildLogFormatUpstream(input interface{}) string {
	cfg, ok := input.(ingressConfig.Configuration)
	if !ok {
		glog.Errorf("error  an ingress.buildLogFormatUpstream type but %T was returned", input)
	}

	return cfg.BuildLogFormatUpstream()
}

func buildNextUpstream(input interface{}) string {
	nextUpstream, ok := input.(string)
	if !ok {
		glog.Errorf("expected an string type but %T was returned", input)
	}

	parts := strings.Split(nextUpstream, " ")

	nextUpstreamCodes := make([]string, 0, len(parts))
	for _, v := range parts {
		if v != "" && v != "non_idempotent" {
			nextUpstreamCodes = append(nextUpstreamCodes, v)
		}
	}

	return strings.Join(nextUpstreamCodes, " ")
}
