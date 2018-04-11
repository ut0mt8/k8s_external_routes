package main

import (
	"context"
	"fmt"
	"github.com/ericchiang/k8s"
	corev1 "github.com/ericchiang/k8s/apis/core/v1"
	"github.com/ghodss/yaml"
	"github.com/namsral/flag"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"reflect"
	"runtime"
)

type Config struct {
	kubeConfig string
	syncPeriod int
	debug      bool
}

type Route struct {
	Destination string
	Nexthop     string
	Label       string
}

type RouteAction func(route *netlink.Route) error

func (ra RouteAction) Name() string {
	return runtime.FuncForPC(reflect.ValueOf(ra).Pointer()).Name()
}

var currentRoutes []Route
var config Config
var log = logrus.New()

func setupCloseHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		for sig := range c {
			log.Infof("\r\nCaptured %v, gracefully cleaning routing table...", sig)
			configureRoutes(currentRoutes, netlink.RouteDel)
			log.Infof("All cleaned: Goodbye.")
			os.Exit(1)
		}
	}()
}

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

	var nodes corev1.NodeList
	if err := client.List(context.Background(), "", &nodes); err != nil {
		return nil, fmt.Errorf("Cannot list nodes: %v", err)
	}

	for _, n := range nodes.Items {

		if *n.Spec.PodCIDR == "" {
			log.Debugf("Node : %v have not podCidr, skipping it", *n.Metadata.Name)
			continue
		}

		log.Debugf("Node : %v, podCidr : %v", *n.Metadata.Name, *n.Spec.PodCIDR)
		for _, a := range n.Status.Addresses {

			log.Debugf(" - Address : %v %v", *a.Address, *a.Type)

			if *a.Type == "InternalIP" {

				if *a.Address == "" {
					log.Debugf("Node : %v have not internalIP, skipping it", *n.Metadata.Name)
					continue
				}

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

func configureRoutes(routes []Route, ra RouteAction) {

	for n, route := range routes {
		log.Infof(" - Route #%v, %v %v %v", n, route.Label, route.Destination, route.Nexthop)

		_, dst, err := net.ParseCIDR(route.Destination)
		if err != nil {
			log.Errorf("    Error parsing route destination : %v", err)
		}

		ip := net.ParseIP(route.Nexthop)

		if err := ra(&netlink.Route{Dst: dst, Gw: ip}); err != nil {
			log.Errorf("    Error in %v : %v", ra.Name(), err)
		}

	}

	return
}

func init() {

	flag.StringVar(&config.kubeConfig, "kubeConfig", os.Getenv("HOME")+"/.kube/config", "kubeconfig file to load")
	flag.IntVar(&config.syncPeriod, "syncPeriod", 600, "Period between update")
	flag.BoolVar(&config.debug, "debug", false, "Enable debug messages")

	log.Formatter = &logrus.TextFormatter{
		FullTimestamp: true,
	}
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

	setupCloseHandler()

	// Such idiom permits immediate first tick
	for t := time.Tick(time.Duration(config.syncPeriod) * time.Second); ; <-t {
		log.Infof("New Tick fired")
		currentRoutes, err = getRoutes(client)
		if err != nil {
			log.Errorf("Failed GetRoutes: %v", err)
			continue
		}
		configureRoutes(currentRoutes, netlink.RouteReplace)
	}

}
