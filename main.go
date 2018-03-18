package main

import (
	"context"
	"fmt"
	"github.com/ericchiang/k8s"
	"github.com/ghodss/yaml"
	"github.com/namsral/flag"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"os/exec"
	"reflect"
	"text/template"
	"time"
)

type Config struct {
	kubeConfig   string
	tmplFile     string
	configFile   string
	reloadScript string
	syncPeriod   int
	debug        bool
}

type Route struct {
	Destination string
	Nexthop     string
	Label       string
}

var config Config
var log = logrus.New()

func loadClient(kubeconfigPath string) (*k8s.Client, error) {

	data, err := ioutil.ReadFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("read kubeconfig: %v", err)
	}

	var cfg k8s.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal kubeconfig: %v", err)
	}

	return k8s.NewClient(&cfg)
}

func getRoutes(client *k8s.Client) (routes []Route, err error) {

	nodes, err := client.CoreV1().ListNodes(context.Background())
	if err != nil {
		return nil, fmt.Errorf("Cannot list nodes: %v", err)
	}

	for _, n := range nodes.Items {

		log.Debugf("Node : %v, podCidr : %v", *n.Metadata.Name, *n.Spec.PodCIDR)
		for _, a := range n.Status.Addresses {

			log.Debugf(" - Address : %v %v", *a.Address, *a.Type)

			if *a.Type == "InternalIP" {

				nRoute := Route{
					Destination: *n.Spec.PodCIDR,
					Nexthop:     *a.Address,
					Label:       *n.Metadata.Name,
				}
				routes = append(routes, nRoute)
				log.Debugf("Route OK : %+v", nRoute)
			}
		}
	}

	return routes, nil
}

func configureRoutes(routes []Route, tmplFile string, configFile string) {

	for n, route := range routes {
		log.Infof("Route #%v, %v %v %v", n, route.Label, route.Destination, route.Nexthop)
	}

	t, err := template.ParseFiles(tmplFile)
	if err != nil {
		log.Errorf("Failed to load template file: %v", err)
		return
	}

	w, err := os.Create(configFile)
	if err != nil {
		log.Errorf("Failed to open config file: %v", err)
		return
	}

	conf := make(map[string]interface{})
	conf["routes"] = routes

	err = t.Execute(w, conf)
	if err != nil {
		log.Errorf("Failed to write config file: %v", err)
		return
	} else {
		log.Infof("Write config file: %v", configFile)
	}

	log.Infof("Ready to reload routes")

	out, err := exec.Command(config.reloadScript).CombinedOutput()
	if err != nil {
		log.Errorf("Error reloading routes: %v\n%s", err, out)
	} else {
		log.Infof("Reload script succeed:\n%s", out)
	}

	return
}

func init() {

	flag.StringVar(&config.kubeConfig, "kubeConfig", os.Getenv("HOME")+"/.kube/config", "kubeconfig file to load")
	flag.StringVar(&config.tmplFile, "tmplFile", "config.tmpl", "Template file to load")
	flag.StringVar(&config.configFile, "configFile", "config.conf", "Configuration file to write")
	flag.StringVar(&config.reloadScript, "reloadScript", "./reload.sh", "Reload script to launch")
	flag.IntVar(&config.syncPeriod, "syncPeriod", 600, "Period between update")
	flag.BoolVar(&config.debug, "debug", false, "Enable debug messages")

	log.Formatter = new(logrus.TextFormatter)
	log.Level = logrus.InfoLevel
}

func main() {

	flag.Parse()
	if config.debug {
		log.SetLevel(logrus.DebugLevel)
	}

	client, err := loadClient(config.kubeConfig)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	log.Infof("Initial GetRoutes fired")
	currentRoutes, err := getRoutes(client)
	if err != nil {
		log.Fatalf("Failed initial GetRoutes: %v", err)
	}
	configureRoutes(currentRoutes, config.tmplFile, config.configFile)

	for t := range time.NewTicker(time.Duration(config.syncPeriod) * time.Second).C {

		log.Debugf("GetRoutes fired at %+v", t)
		newRoutes, err := getRoutes(client)
		if err != nil {
			log.Errorf("Failed GetRoutes: %v", err)
		}

		if !reflect.DeepEqual(newRoutes, currentRoutes) {
			log.Infof("Routes have changed, script fired")
			currentRoutes = newRoutes
			configureRoutes(currentRoutes, config.tmplFile, config.configFile)
		}
	}
}
