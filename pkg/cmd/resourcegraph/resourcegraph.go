package resourcegraph

import (
	"fmt"

	"github.com/gonum/graph/encoding/dot"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	"github.com/openshift/cluster-kube-controller-manager-operator/pkg/operator/operatorclient"
	"github.com/openshift/library-go/pkg/operator/resource/resourcegraph"
)

func NewResourceChainCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource-graph",
		Short: "Where do resources come from? Ask your mother.",
		Run: func(cmd *cobra.Command, args []string) {
			resources := Resources()
			g := resources.NewGraph()

			data, err := dot.Marshal(g, resourcegraph.Quote("kube-apiserver-operator"), "", "  ", false)
			if err != nil {
				klog.Fatal(err)
			}
			fmt.Println(string(data))
		},
	}

	return cmd
}

func Resources() resourcegraph.Resources {
	ret := resourcegraph.NewResources()

	payload := resourcegraph.NewResource(resourcegraph.NewCoordinates("", "Payload", "", "cluster")).
		Add(ret)
	installer := resourcegraph.NewResource(resourcegraph.NewCoordinates("", "Installer", "", "cluster")).
		Add(ret)
	user := resourcegraph.NewResource(resourcegraph.NewCoordinates("", "User", "", "cluster")).
		Add(ret)

	cvo := resourcegraph.NewOperator("cluster-version").
		From(payload).
		Add(ret)
	kasOperator := resourcegraph.NewOperator("kube-apiserver").
		From(cvo).
		Add(ret)
	kcmOperator := resourcegraph.NewOperator("kube-controller-manager").
		From(cvo).
		Add(ret)
	networkOperator := resourcegraph.NewOperator("network").
		From(cvo).
		Add(ret)
	ingressOperator := resourcegraph.NewOperator("ingress").
		From(cvo).
		Add(ret)
	serviceCAOperator := resourcegraph.NewOperator("service-ca").
		From(cvo).
		Add(ret)

	// config.openshift.io
	networkConfig := resourcegraph.NewConfig("networks").
		From(user).
		From(networkOperator).
		Add(ret)
	infrastructureConfig := resourcegraph.NewConfig("infrastructures").
		From(user).
		From(installer).
		Add(ret)
	apiserversConfig := resourcegraph.NewConfig("apiservers").
		From(user).
		From(installer).
		Add(ret)
	proxiesConfig := resourcegraph.NewConfig("proxies").
		From(user).
		From(installer).
		Add(ret)
	featureGatesConfig := resourcegraph.NewConfig("featuregates").
		From(user).
		From(installer).
		Add(ret)

	// key for signing service account tokens
	initialServiceAccountKey := resourcegraph.NewSecret(operatorclient.GlobalUserSpecifiedConfigNamespace, "initial-service-account-private-key").
		Note("Static").
		From(installer).
		Add(ret)
	nextServiceAccountKey := resourcegraph.NewSecret(operatorclient.OperatorNamespace, "next-service-account-private-key").
		Note("Static").
		From(kcmOperator).
		Add(ret)
	serviceAccountKey := resourcegraph.NewSecret(operatorclient.TargetNamespace, "service-account-private-key").
		Note("Synchronized").
		From(initialServiceAccountKey).
		From(nextServiceAccountKey).
		Add(ret)
	// sa-token-signing-certs public keys
	_ = resourcegraph.NewConfigMap(operatorclient.GlobalMachineSpecifiedConfigNamespace, "sa-token-signing-certs").
		Note("Synchronized").
		From(nextServiceAccountKey).
		Add(ret)

	// client cert/key
	controlPlaneSignerCA := resourcegraph.NewSecret("openshift-kube-apiserver-operator", "kube-control-plane-signer").
		Note("Rotated").
		From(kasOperator).
		Add(ret)
	kasCertKey := resourcegraph.NewSecret(operatorclient.GlobalMachineSpecifiedConfigNamespace, "kube-controller-manager-client-cert-key").
		Note("Rotated").
		From(controlPlaneSignerCA).
		Add(ret)
	clientCertKey := resourcegraph.NewSecret(operatorclient.TargetNamespace, "kube-controller-manager-client-cert-key").
		Note("Synchronized").
		From(kasCertKey).
		Add(ret)

	// client CA bundle
	clientCA := resourcegraph.NewConfigMap("openshift-kube-apiserver", "client-ca").
		Note("Rotated").
		From(kasOperator).
		Add(ret)
	clientCAManaged := resourcegraph.NewConfigMap(operatorclient.GlobalMachineSpecifiedConfigNamespace, "kube-apiserver-client-ca").
		Note("Synchronized").
		From(clientCA).
		Add(ret)
	clientCATarget := resourcegraph.NewConfigMap(operatorclient.TargetNamespace, "client-ca").
		Note("Synchronized").
		From(clientCAManaged).
		Add(ret)

	// aggregator client CA bundle
	aggregatorClientCA := resourcegraph.NewConfigMap(operatorclient.GlobalMachineSpecifiedConfigNamespace, "kube-apiserver-aggregator-client-ca").
		Note("Rotated").
		From(kasOperator).
		Add(ret)
	aggregatorClientCATarget := resourcegraph.NewConfigMap(operatorclient.TargetNamespace, "aggregator-client-ca").
		Note("Synchronized").
		From(aggregatorClientCA).
		Add(ret)

	// localhost client token for cert-syncer and recovery-controller
	localhostRecoveryClientToken := resourcegraph.NewSecret(operatorclient.TargetNamespace, "localhost-recovery-client-token").
		Note("Static").
		From(kcmOperator).
		Add(ret)

	// CSR
	managedCSRSignerSigner := resourcegraph.NewSecret(operatorclient.OperatorNamespace, "csr-signer-signer").
		Note("Rotated").
		From(kcmOperator).
		Add(ret)
	managedCSRSignerSignerCA := resourcegraph.NewConfigMap(operatorclient.OperatorNamespace, "csr-controller-signer-ca").
		Note("Rotated").
		From(managedCSRSignerSigner).
		Add(ret)
	managedCSRSigner := resourcegraph.NewSecret(operatorclient.OperatorNamespace, "csr-signer").
		Note("Rotated").
		From(managedCSRSignerSigner).
		Add(ret)
	strippedSigner := resourcegraph.NewSecret(operatorclient.TargetNamespace, "csr-signer").
		Note("Reduced").
		From(managedCSRSigner).
		Add(ret)
	managedCSRSignerCA := resourcegraph.NewConfigMap(operatorclient.OperatorNamespace, "csr-signer-ca").
		Note("Rotated").
		From(managedCSRSigner).
		Add(ret)
	operatorCSRCA := resourcegraph.NewConfigMap(operatorclient.OperatorNamespace, "csr-controller-ca").
		Note("Unioned").
		From(managedCSRSignerCA).
		From(managedCSRSignerSignerCA).
		Add(ret)
	// this is a destination for KAS
	_ = resourcegraph.NewConfigMap(operatorclient.GlobalMachineSpecifiedConfigNamespace, "csr-controller-ca").
		Note("Synchronized").
		From(operatorCSRCA).
		Add(ret)

	// serviceaccount-ca CA bundle
	kasCA := resourcegraph.NewConfigMap(operatorclient.GlobalMachineSpecifiedConfigNamespace, "kube-apiserver-server-ca").
		Note("Rotated").
		From(kasOperator).
		Add(ret)
	routerWildcardCA := resourcegraph.NewConfigMap(operatorclient.GlobalMachineSpecifiedConfigNamespace, "default-ingress-cert").
		Note("Static").
		From(ingressOperator).
		Add(ret)
	saCA := resourcegraph.NewConfigMap(operatorclient.TargetNamespace, "serviceaccount-ca").
		Note("Unioned").
		From(routerWildcardCA).
		From(kasCA).
		Add(ret)

	// service-ca signing-key secret  and CA config map
	servicecaSigningKey := resourcegraph.NewSecret("openshift-service-ca", "signing-key").
		Note("Rotated").
		From(serviceCAOperator).
		Add(ret)
	servicecaSigningCA := resourcegraph.NewConfigMap("openshift-service-ca", "signing-cabundle").
		Note("Rotated").
		From(servicecaSigningKey).
		Add(ret)
	servicecaSigningCAManaged := resourcegraph.NewConfigMap("openshift-config-managed", "service-ca").
		Note("Synchronized").
		From(servicecaSigningCA).
		Add(ret)
	servicecaSigningCATarget := resourcegraph.NewConfigMap(operatorclient.TargetNamespace, "service-ca").
		Note("Synchronized").
		From(servicecaSigningCAManaged).
		Add(ret)

	// KCM serving cert
	serviceCAController := resourcegraph.NewResource(resourcegraph.NewCoordinates("apps", "deployments", "openshift-service-ca", "service-ca")).
		From(servicecaSigningKey).
		From(serviceCAOperator).
		Add(ret)
	servingCert := resourcegraph.NewSecret(operatorclient.TargetNamespace, "serving-cert").
		Note("Rotated").
		From(serviceCAController).
		Add(ret)

	// observedConfig
	config := resourcegraph.NewConfigMap(operatorclient.TargetNamespace, "config").
		Note("Managed").
		From(infrastructureConfig). // cloud provider
		From(networkConfig).        // service cidr for controllers
		From(apiserversConfig).     // TLS security profile
		From(proxiesConfig).        // proxy env in KCM pod
		From(featureGatesConfig).   // feature gates
		Add(ret)

	// and finally our target pod
	_ = resourcegraph.NewResource(resourcegraph.NewCoordinates("", "pods", operatorclient.TargetNamespace, "kube-controller-manager")).
		From(serviceAccountKey).
		From(clientCertKey).
		From(saCA).
		From(clientCATarget).
		From(aggregatorClientCATarget).
		From(servingCert).
		From(servicecaSigningCATarget).
		From(localhostRecoveryClientToken).
		From(strippedSigner).
		From(config).
		Add(ret)

	return ret
}
