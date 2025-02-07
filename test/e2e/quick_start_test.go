// +build e2e

package e2e

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	hyperapi "github.com/openshift/hypershift/api"
	hyperv1 "github.com/openshift/hypershift/api/v1alpha1"
	cmdcluster "github.com/openshift/hypershift/cmd/cluster"
)

type QuickStartOptions struct {
	AWSCredentialsFile string
	PullSecretFile     string
	ReleaseImage       string
	ArtifactDir        string
	BaseDomain         string
}

func NewQuickStartOptions(globalOptions *GlobalTestOptions) QuickStartOptions {
	return QuickStartOptions{
		AWSCredentialsFile: globalOptions.AWSCredentialsFile,
		PullSecretFile:     globalOptions.PullSecretFile,
		ReleaseImage:       globalOptions.LatestReleaseImage,
		ArtifactDir:        globalOptions.ArtifactDir,
		BaseDomain:         globalOptions.BaseDomain,
	}
}

// TestQuickStart implements a test that mimics the operation described in the
// HyperShift quick start (creating a basic guest cluster).
//
// This test is meant to provide a first, fast signal to detect regression; it
// is recommended to use it as a PR blocker test.
func TestQuickStart(t *testing.T) {
	ctx, cancel := context.WithCancel(GlobalTestContext)
	defer cancel()

	opts := NewQuickStartOptions(GlobalOptions)
	t.Logf("Testing OCP release image %s", opts.ReleaseImage)

	g := NewWithT(t)

	client, err := crclient.New(ctrl.GetConfigOrDie(), crclient.Options{Scheme: hyperapi.Scheme})
	g.Expect(err).NotTo(HaveOccurred(), "failed to create kube client")

	// Create a namespace in which to place hostedclusters
	namespace := GenerateNamespace(t, ctx, client, "e2e-clusters-")
	name := SimpleNameGenerator.GenerateName("example-")

	// Define the cluster we'll be testing
	hostedCluster := &hyperv1.HostedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace.Name,
			Name:      name,
		},
	}

	// Ensure we clean up after the test
	defer func() {
		DestroyCluster(t, context.Background(), &cmdcluster.DestroyOptions{
			Namespace:          hostedCluster.Namespace,
			Name:               hostedCluster.Name,
			AWSCredentialsFile: opts.AWSCredentialsFile,
		}, opts.ArtifactDir)
		DeleteNamespace(t, context.Background(), client, namespace.Name)
	}()

	// Create the cluster
	createClusterOpts := cmdcluster.Options{
		Namespace:          hostedCluster.Namespace,
		Name:               hostedCluster.Name,
		ReleaseImage:       opts.ReleaseImage,
		PullSecretFile:     opts.PullSecretFile,
		AWSCredentialsFile: opts.AWSCredentialsFile,
		// TODO: generate a key on the fly
		SSHKeyFile:       "",
		NodePoolReplicas: 2,
		Region:           "us-east-1",
		InstanceType:     "m4.large",
		BaseDomain:       opts.BaseDomain,
	}
	err = cmdcluster.CreateCluster(ctx, createClusterOpts)
	g.Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

	// Get the newly created cluster
	err = client.Get(ctx, crclient.ObjectKeyFromObject(hostedCluster), hostedCluster)
	g.Expect(err).NotTo(HaveOccurred(), "failed to get hostedcluster")
	t.Logf("Created hostedcluster %s/%s", hostedCluster.Namespace, hostedCluster.Name)

	// Perform some very basic assertions about the guest cluster
	guestClient := WaitForGuestClient(t, ctx, client, hostedCluster)

	WaitForReadyNodes(t, ctx, guestClient, hostedCluster)

	WaitForReadyClusterOperators(t, ctx, guestClient, hostedCluster)
}
