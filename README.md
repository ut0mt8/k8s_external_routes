# k8s_external_routes
small go program to get pod cidr routes from k8s api-server. The goal is to inject theses routes in any external device to provide direct connectivity.

This obviously work only on host-gw network k8s mode.
