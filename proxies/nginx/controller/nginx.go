package controller

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/caicloud/canary-release/proxies/nginx/config"
	"github.com/caicloud/canary-release/proxies/nginx/template"
	"github.com/mitchellh/go-ps"
	log "github.com/zoumo/logdog"

	"k8s.io/apimachinery/pkg/util/wait"
)

var (
	tmplPath = "/etc/nginx/template/nginx.tmpl"
	cfgPath  = "/etc/nginx/nginx.conf"
	binary   = "/usr/sbin/nginx"
)

// NginxController ...
type NginxController struct {
	binary   string
	cmdArgs  []string
	template *template.Template
}

// NewNginxController returns a new NginxController
func NewNginxController() *NginxController {
	n := &NginxController{
		binary: binary,
	}
	var onChange func()
	onChange = func() {
		tmpl, err := template.NewTemplate(tmplPath, onChange)
		if err != nil {
			log.Errorf(`
-------------------------------------------------------------------------------
Error loading new template : %v
-------------------------------------------------------------------------------
`, err)
			return
		}
		n.template.Close()
		n.template = tmpl
		log.Info("New Nginx template loaded")
	}

	tmpl, err := template.NewTemplate(tmplPath, onChange)
	if err != nil {
		log.Error("Invalid Nginx template", log.Fields{"err": err})
	}
	n.template = tmpl
	return n
}

// Start ...
func (n *NginxController) Start() {

	done := make(chan error, 1)
	cmd := exec.Command(n.binary, "-c", cfgPath)
	// put nginx in another process group to prevent it
	// to receive signals meant for the controller
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	log.Info("Starting nginx process...")
	n.start(cmd, done)

	for {
		err := <-done
		if exitError, ok := err.(*exec.ExitError); ok {
			waitStatus := exitError.Sys().(syscall.WaitStatus)
			log.Warnf(`
-------------------------------------------------------------------------------
NGINX master process died (%v): %v
-------------------------------------------------------------------------------
`, waitStatus.ExitStatus(), err)
		}
		_ = cmd.Process.Release()
		cmd = exec.Command(n.binary, "-c", cfgPath)

		_ = wait.PollInfinite(1*time.Second, func() (bool, error) {
			conn, err := net.DialTimeout("tcp", "127.0.0.1:80", 1*time.Second)
			if err != nil {
				return true, nil
			}
			_ = conn.Close()
			return false, nil
		})

		n.start(cmd, done)

	}

}

func (n *NginxController) start(cmd *exec.Cmd, done chan error) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Errorf("nginx error: %v", err)
		done <- err
		return
	}

	n.cmdArgs = cmd.Args

	go func() {
		done <- cmd.Wait()
	}()
}

// Stop ...
func (n *NginxController) Stop() error {
	cmd := exec.Command(n.binary, "-c", cfgPath, "-s", "quit")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}
	waitForNginxShutdown()
	log.Info("Nginx process has stopped")
	return nil
}

// testTemplate checks if the NGINX configuration inside the byte array is valid
// running the command "nginx -t" using a temporal file.
func (n *NginxController) testTemplate(cfg []byte) error {
	if len(cfg) == 0 {
		return fmt.Errorf("invalid nginx configuration (empty)")
	}
	tmpfile, err := ioutil.TempFile("", "nginx-cfg")
	if err != nil {
		return err
	}
	defer func() {
		_ = tmpfile.Close()
	}()
	err = ioutil.WriteFile(tmpfile.Name(), cfg, 0644)
	if err != nil {
		return err
	}

	out, err := exec.Command(n.binary, "-t", "-c", tmpfile.Name()).CombinedOutput()
	if err != nil {
		// this error is different from the rest because it must be clear why nginx is not working
		oe := fmt.Sprintf(`
-------------------------------------------------------------------------------
Error: %v
%v
-------------------------------------------------------------------------------
`, err, string(out))
		return errors.New(oe)
	}

	_ = os.Remove(tmpfile.Name())
	return nil
}

// OnUpdate is called by proxy.sync periodically to keep the configuration in sync
func (n *NginxController) OnUpdate(cfg config.TemplateConfig) error {

	backlogSize := sysctlSomaxconn()

	wp, err := strconv.Atoi(cfg.Cfg.WorkerProcesses)
	if err != nil {
		wp = 1
	}

	maxOpenFiles := (sysctlFSFileMax() / wp) - 1024
	if maxOpenFiles < 1024 {
		maxOpenFiles = 1024
	}

	cfg.BacklogSize = backlogSize
	cfg.MaxOpenFiles = maxOpenFiles
	cfg.IsIPV6Enabled = true
	cfg.Cfg.EnableVtsStatus = true

	content, err := n.template.Write(cfg)
	if err != nil {
		return err
	}

	err = n.testTemplate(content)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(cfgPath, content, 0644)
	if err != nil {
		return err
	}

	o, err := exec.Command(n.binary, "-s", "reload", "-c", cfgPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v\n%v", err, string(o))
	}

	return nil
}

// isNginxRunning returns true if a process with the name 'nginx' is found
func isNginxProcessPresent() bool {
	processes, _ := ps.Processes()
	for _, p := range processes {
		if p.Executable() == "nginx" {
			return true
		}
	}
	return false
}

func waitForNginxShutdown() {
	_ = wait.PollImmediateInfinite(1*time.Second, func() (bool, error) {
		if !isNginxProcessPresent() {
			return true, nil
		}
		return false, nil
	})
}
