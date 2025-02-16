package gcwatchercontroller

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	prometheusapi "github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/transport"

	"github.com/openshift/cluster-kube-controller-manager-operator/pkg/operator/operatorclient"
)

func newPrometheusClient(ctx context.Context, configMapClient corev1client.ConfigMapsGetter) (prometheusv1.API, error) {
	host := "thanos-querier.openshift-monitoring.svc"

	saToken, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return nil, fmt.Errorf("error reading service account token: %w", err)
	}

	routerCAConfigMap, err := configMapClient.ConfigMaps(operatorclient.GlobalMachineSpecifiedConfigNamespace).Get(ctx, "service-ca", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	bundlePEM := []byte(routerCAConfigMap.Data["ca-bundle.crt"])

	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(bundlePEM)

	client, err := prometheusapi.NewClient(prometheusapi.Config{
		Address: "https://" + net.JoinHostPort(host, "9091"),
		RoundTripper: transport.NewBearerAuthRoundTripper(
			string(saToken),
			&http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout: 10 * time.Second,
				TLSClientConfig: &tls.Config{
					RootCAs:    roots,
					ServerName: host,
				},
			},
		),
	})
	if err != nil {
		return nil, err
	}

	return prometheusv1.NewAPI(client), nil
}
