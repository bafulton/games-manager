package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bafulton/games-manager/test/utils"
)

var (
	// projectImage is the controller image under test. CI builds it before
	// the suite runs and sets E2E_SKIP_IMAGE_BUILD, so the suite only loads it
	// into Kind.
	projectImage   = envOr("E2E_IMAGE", "example.com/games-manager:e2e")
	skipImageBuild = os.Getenv("E2E_SKIP_IMAGE_BUILD") == "true"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// TestE2E installs the Helm chart into a Kind cluster and runs a real game
// server through it. It needs Kind, Helm and kubectl.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting games-manager e2e suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	if !skipImageBuild {
		By("building the controller image")
		_, err := utils.Run(exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage)))
		Expect(err).NotTo(HaveOccurred(), "Failed to build the controller image")
	}

	By("loading the controller image into Kind")
	Expect(utils.LoadImageToKindClusterWithName(projectImage)).To(Succeed(), "Failed to load the image into Kind")
})
