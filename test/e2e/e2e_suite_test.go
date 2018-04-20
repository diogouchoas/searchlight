package e2e

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	logs "github.com/appscode/go/log/golog"
	api "github.com/appscode/searchlight/apis/monitoring/v1alpha1"
	cs "github.com/appscode/searchlight/client/clientset/versioned"
	"github.com/appscode/searchlight/pkg/icinga"
	"github.com/appscode/searchlight/pkg/operator"
	"github.com/appscode/searchlight/pkg/plugin"
	"github.com/appscode/searchlight/test/e2e/framework"
	. "github.com/appscode/searchlight/test/e2e/matcher"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var (
	provider           string
	storageClass       string
	providedIcinga     string
	providedController bool
)

func init() {
	flag.StringVar(&provider, "provider", "minikube", "Kubernetes cloud provider")
	flag.StringVar(&storageClass, "storageclass", "", "Kubernetes StorageClass name")
	flag.StringVar(&providedIcinga, "icinga-reference", "", "Running Icinga reference")
	flag.BoolVar(&providedController, "provided-controller", false, "Enable this for provided controller")
}

const (
	TIMEOUT = 20 * time.Minute
)

var (
	op   *operator.Operator
	root *framework.Framework
)

func TestE2e(t *testing.T) {
	logs.InitLogs()
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(TIMEOUT)

	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "e2e Suite", []Reporter{junitReporter})
}

var _ = BeforeSuite(func() {
	// Kubernetes config
	kubeconfigPath := filepath.Join(homedir.HomeDir(), ".kube/config")
	By("Using kubeconfig from " + kubeconfigPath)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	Expect(err).NotTo(HaveOccurred())
	// Clients
	Expect(err).NotTo(HaveOccurred())
	kubeClient := kubernetes.NewForConfigOrDie(config)
	apiExtKubeClient := crd_cs.NewForConfigOrDie(config)
	extClient := cs.NewForConfigOrDie(config)
	// Framework
	root = framework.New(kubeClient, apiExtKubeClient, extClient, nil, provider, storageClass)

	framework.PrintSeparately("Using namespace " + root.Namespace())

	// Create namespace
	err = root.CreateNamespace()
	Expect(err).NotTo(HaveOccurred())

	var slService *core.Service
	if providedIcinga == "" {
		// Create Searchlight deployment
		slDeployment := root.Invoke().DeploymentSearchlight()
		err = root.CreateDeployment(slDeployment)
		Expect(err).NotTo(HaveOccurred())
		By("Waiting for Running pods")
		root.EventuallyDeployment(slDeployment.ObjectMeta).Should(HaveRunningPods(*slDeployment.Spec.Replicas))
		// Create Searchlight service
		slService = root.Invoke().ServiceSearchlight()
		err = root.CreateService(slService)
		Expect(err).NotTo(HaveOccurred())
		root.EventuallyServiceLoadBalancer(slService.ObjectMeta, "icinga").Should(BeTrue())

	} else {
		parts := strings.Split(providedIcinga, "@")
		om := metav1.ObjectMeta{
			Name:      parts[0],
			Namespace: parts[1],
		}
		slService = &core.Service{ObjectMeta: om}
	}

	// Get Icinga Ingress Hostname
	endpoint, err := root.GetServiceEndpoint(slService.ObjectMeta, "icinga")
	Expect(err).NotTo(HaveOccurred())

	// Icinga Config
	cfg := &icinga.Config{
		Endpoint: fmt.Sprintf("https://%v/v1", endpoint),
		CACert:   nil,
	}

	if providedIcinga == "" {
		cfg.BasicAuth.Username = ICINGA_API_USER
		cfg.BasicAuth.Password = ICINGA_API_PASSWORD
	} else {
		c, err := root.GetIcingaApiAuth(slService.ObjectMeta)
		Expect(err).NotTo(HaveOccurred())
		cfg.BasicAuth.Username = c.BasicAuth.Username
		cfg.BasicAuth.Password = c.BasicAuth.Password
	}

	fmt.Println(cfg.Endpoint)
	fmt.Println(cfg.BasicAuth.Username)
	fmt.Println(cfg.BasicAuth.Password)

	// Icinga Client
	icingaClient := icinga.NewClient(*cfg)
	root = root.SetIcingaClient(icingaClient)
	root.EventuallyIcingaAPI().Should(Succeed())

	icingawebEndpoint, err := root.GetServiceEndpoint(slService.ObjectMeta, "ui")
	Expect(err).NotTo(HaveOccurred())
	fmt.Println()
	fmt.Println("Icingaweb2:     ", fmt.Sprintf("http://%v/", icingawebEndpoint))
	fmt.Println()

	if !providedController {
		opc := &operator.OperatorConfig{
			Config: operator.Config{
				MaxNumRequeues: 3,
				NumThreads:     3,
				Verbosity:      "6",
			},
			KubeClient:   kubeClient,
			CRDClient:    apiExtKubeClient,
			ExtClient:    extClient,
			IcingaClient: icingaClient,
		}
		// Controller
		op, err = opc.New()
		Expect(err).NotTo(HaveOccurred())
		go op.RunWatchers(nil)
	}

	plugins := []*api.SearchlightPlugin{
		plugin.GetComponentStatusPlugin(),
		plugin.GetNodeExistsPlugin(),
		plugin.GetPodExistsPlugin(),
		plugin.GetNodeStatusPlugin(),
		plugin.GetNodeVolumePlugin(),
		plugin.GetPodStatusPlugin(),
		plugin.GetPodVolumePlugin(),
	}

	for _, p := range plugins {
		extClient.MonitoringV1alpha1().SearchlightPlugins().Create(p)
	}

})

var _ = AfterSuite(func() {
	root.CleanPodAlert()
	root.CleanNodeAlert()
	root.CleanClusterAlert()
	err := root.DeleteNamespace()
	Expect(err).NotTo(HaveOccurred())
	framework.PrintSeparately("Deleted namespace")
})
