package controller

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Game validation", func() {
	It("rejects an architecture in nodeSelector alongside arch", func() {
		game := valheim()
		game.Name = "arch-twice"
		game.Spec.NodeSelector = map[string]string{"kubernetes.io/arch": "arm64"}
		err := k8sClient.Create(context.Background(), game, client.DryRunAll)
		Expect(err).To(MatchError(ContainSubstring("set the architecture with arch")))
	})
})

// The examples are documentation, so make sure the CRDs accept them.
var _ = Describe("Examples", func() {
	It("are accepted by the CRDs", func() {
		ctx := context.Background()
		paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.yaml"))
		Expect(err).NotTo(HaveOccurred())
		Expect(paths).NotTo(BeEmpty())

		for _, path := range paths {
			data, err := os.ReadFile(path)
			Expect(err).NotTo(HaveOccurred())
			decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
			for {
				obj := &unstructured.Unstructured{}
				if err := decoder.Decode(&obj.Object); err == io.EOF {
					break
				} else {
					Expect(err).NotTo(HaveOccurred(), path)
				}
				if obj.GetKind() != "Game" {
					continue // GameServers need their namespace and secrets
				}
				// Renamed and dry-run, so an example can't collide with the
				// controller tests' own Games and skip validation.
				obj.SetName(obj.GetName() + "-example")
				Expect(k8sClient.Create(ctx, obj, client.DryRunAll)).To(Succeed(), path)
			}
		}
	})
})
