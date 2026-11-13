package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bafulton/games-manager/test/utils"
)

const (
	// namespace the chart installs the controller into.
	namespace = "games-manager"
	// gamesNamespace holds the test's GameServers. It enforces the
	// restricted Pod Security Standard, so the controller's default pod and
	// container security contexts are checked too.
	gamesNamespace = "games-e2e"
)

// sleeperGame is a stand-in game: busybox sleeping, with a world volume. It
// records when its world was first created in /world/born, so a server
// recreated on an existing world can be told apart from a fresh one.
const sleeperGame = `
apiVersion: games.homelab.io/v1alpha1
kind: Game
metadata:
  name: sleeper
spec:
  image: busybox:1.37.0
  command: [sh, -c, "[ -f /world/born ] || cat /proc/sys/kernel/random/uuid > /world/born; exec sleep 3600"]
  ports:
    - name: game
      containerPort: 7777
  connection:
    type: Hostname
    hostname:
      domain: e2e.example.com
  sizes:
    - name: small
      resources:
        requests: {cpu: 10m, memory: 16Mi}
        limits: {memory: 32Mi}
  defaultSize: small
  versions:
    - name: v1
      env: [{name: SLEEPER_VERSION, value: "1"}]
    - name: v2
      env: [{name: SLEEPER_VERSION, value: "2"}]
  defaultVersion: v1
  storage:
    size: 64Mi
    mountPath: /world
    # Kind's local-path provisioner.
    storageClassName: standard
  readinessProbe:
    exec:
      command: [test, -f, /world/born]
    periodSeconds: 2
  terminationGracePeriodSeconds: 5
`

func applyManifest(manifest string) {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

func createServer(name string) {
	applyManifest(fmt.Sprintf(`
apiVersion: games.homelab.io/v1alpha1
kind: GameServer
metadata:
  name: %s
  namespace: %s
spec:
  game: sleeper
  owner: e2e
`, name, gamesNamespace))
}

func get(args ...string) (string, error) {
	return utils.Kubectl(append([]string{"get", "-n", gamesNamespace}, args...)...)
}

func expectPhase(name, phase string, timeout time.Duration) {
	EventuallyWithOffset(1, func(g Gomega) {
		got, err := get("gameserver", name, "-o", "jsonpath={.status.phase}")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(got).To(Equal(phase))
	}, timeout, 2*time.Second).Should(Succeed())
}

func expectGone(kind, name string) {
	EventuallyWithOffset(1, func(g Gomega) {
		out, err := get(kind, name, "--ignore-not-found", "-o", "name")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(out).To(BeEmpty(), "%s/%s should be gone", kind, name)
	}, 2*time.Minute, 2*time.Second).Should(Succeed())
}

func setRunning(name string, running bool) {
	_, err := utils.Kubectl("patch", "gameserver", name, "-n", gamesNamespace,
		"--type", "merge", "-p", fmt.Sprintf(`{"spec":{"running":%t}}`, running))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

// born reads when alice's world was created.
func born() string {
	out, err := utils.Kubectl("exec", "-n", gamesNamespace, "alice-0", "--", "cat", "/world/born")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return out
}

var _ = Describe("games-manager", Ordered, func() {
	BeforeAll(func() {
		repository, tag, _ := strings.Cut(projectImage, ":")

		By("installing the chart")
		_, err := utils.Run(exec.Command("helm", "install", "games-manager", "chart",
			"--namespace", namespace, "--create-namespace",
			"--set", "image.repository="+repository,
			"--set", "image.tag="+tag,
			"--set", "image.pullPolicy=Never",
			"--wait", "--timeout", "2m"))
		Expect(err).NotTo(HaveOccurred(), "Failed to install the chart")

		By("creating a restricted namespace for game servers")
		_, err = utils.Kubectl("create", "ns", gamesNamespace)
		Expect(err).NotTo(HaveOccurred())
		_, err = utils.Kubectl("label", "ns", gamesNamespace, "pod-security.kubernetes.io/enforce=restricted")
		Expect(err).NotTo(HaveOccurred())

		applyManifest(sleeperGame)
	})

	AfterEach(func() {
		if !CurrentSpecReport().Failed() {
			return
		}
		for _, args := range [][]string{
			{"logs", "-n", namespace, "deploy/games-manager", "--tail=100"},
			{"get", "-n", gamesNamespace, "gameservers,statefulsets,pods,services,pvc", "-o", "wide"},
			{"get", "events", "-n", gamesNamespace, "--sort-by=.lastTimestamp"},
		} {
			out, _ := utils.Kubectl(args...)
			_, _ = fmt.Fprintf(GinkgoWriter, "kubectl %s:\n%s\n", strings.Join(args, " "), out)
		}
	})

	It("runs a game server", func() {
		createServer("alice")
		// Generous: the first server pulls busybox.
		expectPhase("alice", "Running", 3*time.Minute)

		address, err := get("gameserver", "alice", "-o", "jsonpath={.status.address}")
		Expect(err).NotTo(HaveOccurred())
		Expect(address).To(Equal("alice.e2e.example.com"))

		const routerAnnotation = `jsonpath={.metadata.annotations.mc-router\.itzg\.me/externalServerName}`
		annotation, err := get("service", "alice", "-o", routerAnnotation)
		Expect(err).NotTo(HaveOccurred())
		Expect(annotation).To(Equal("alice.e2e.example.com"))
	})

	It("stops it with running: false, and starts it again", func() {
		setRunning("alice", false)
		expectPhase("alice", "Stopped", time.Minute)
		expectGone("pod", "alice-0")

		setRunning("alice", true)
		expectPhase("alice", "Running", time.Minute)
	})

	It("keeps the world when a server is deleted, and reuses it", func() {
		first := born()

		_, err := utils.Kubectl("delete", "gameserver", "alice", "-n", gamesNamespace, "--wait")
		Expect(err).NotTo(HaveOccurred())

		By("garbage-collecting the StatefulSet and Service, but not the PVC")
		expectGone("statefulset", "alice")
		expectGone("service", "alice")
		_, err = get("pvc", "alice-world")
		Expect(err).NotTo(HaveOccurred(), "the world PVC must survive the server")

		By("recreating the server on its old world")
		createServer("alice")
		expectPhase("alice", "Running", time.Minute)
		Expect(born()).To(Equal(first), "a recreated server should find its old world")
	})

	It("upgrades a server to another version on the same world", func() {
		version, err := get("gameserver", "alice", "-o", "jsonpath={.spec.version}")
		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(Equal("v1"), "the server should have been given the default version")
		first := born()

		_, err = utils.Kubectl("patch", "gameserver", "alice", "-n", gamesNamespace,
			"--type", "merge", "-p", `{"spec":{"version":"v2"}}`)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			out, err := utils.Kubectl("exec", "-n", gamesNamespace, "alice-0", "--", "printenv", "SLEEPER_VERSION")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).To(Equal("2"))
		}, 2*time.Minute, 2*time.Second).Should(Succeed())
		expectPhase("alice", "Running", time.Minute)
		Expect(born()).To(Equal(first), "an upgraded server should keep its world")
	})

	It("removes the world when a purged server is deleted", func() {
		createServer("bob")
		expectPhase("bob", "Running", time.Minute)

		_, err := utils.Kubectl("annotate", "gameserver", "bob", "-n", gamesNamespace, "games.homelab.io/purge=true")
		Expect(err).NotTo(HaveOccurred())
		_, err = utils.Kubectl("delete", "gameserver", "bob", "-n", gamesNamespace, "--wait")
		Expect(err).NotTo(HaveOccurred())

		expectGone("pvc", "bob-world")
	})
})
