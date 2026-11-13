package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gamesv1alpha1 "github.com/bafulton/games-manager/api/v1alpha1"
)

const testNamespace = "default"

var _ = Describe("GameServer Controller", func() {
	ctx := context.Background()

	var reconciler *GameServerReconciler

	// reconcileUntilSettled runs Reconcile a few times, as the manager would
	// after each write: adding the finalizer and assigning a slot each end
	// one pass early.
	reconcileUntilSettled := func(name string) {
		for range 3 {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name, Namespace: testNamespace},
			})
			Expect(err).NotTo(HaveOccurred())
		}
	}

	get := func(obj client.Object, name string) {
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, obj)).To(Succeed())
	}

	BeforeEach(func() {
		reconciler = &GameServerReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), APIReader: k8sClient}
		for _, g := range []*gamesv1alpha1.Game{valheim(), minecraft()} {
			err := k8sClient.Create(ctx, g)
			if !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		}
	})

	AfterEach(func() {
		servers := &gamesv1alpha1.GameServerList{}
		Expect(k8sClient.List(ctx, servers)).To(Succeed())
		for i := range servers.Items {
			s := &servers.Items[i]
			s.Finalizers = nil
			Expect(k8sClient.Update(ctx, s)).To(Succeed())
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, s))).To(Succeed())
		}
		Expect(k8sClient.DeleteAllOf(ctx, &corev1.PersistentVolumeClaim{}, client.InNamespace(testNamespace))).To(Succeed())
		Expect(k8sClient.DeleteAllOf(ctx, &appsv1.StatefulSet{}, client.InNamespace(testNamespace))).To(Succeed())
	})

	It("builds a world PVC, StatefulSet and Service for a server", func() {
		s := server("alice", "minecraft")
		s.Namespace = testNamespace
		Expect(k8sClient.Create(ctx, s)).To(Succeed())
		reconcileUntilSettled("alice")

		pvc := &corev1.PersistentVolumeClaim{}
		get(pvc, "alice-world")
		Expect(pvc.OwnerReferences).To(BeEmpty(), "the world must outlive the GameServer")

		sts := &appsv1.StatefulSet{}
		get(sts, "alice")
		Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
		Expect(sts.OwnerReferences).To(HaveLen(1))

		svc := &corev1.Service{}
		get(svc, "alice")
		Expect(svc.Annotations).To(HaveKeyWithValue(mcRouterAnnotation, "alice.mc.example.com"))

		get(s, "alice")
		Expect(s.Finalizers).To(ContainElement(finalizer))
		Expect(s.Status.Address).To(Equal("alice.mc.example.com"))
		Expect(s.Status.Phase).To(Equal(gamesv1alpha1.PhaseStarting))
	})

	It("stops a server by scaling it to zero", func() {
		s := server("alice", "minecraft")
		s.Namespace = testNamespace
		Expect(k8sClient.Create(ctx, s)).To(Succeed())
		reconcileUntilSettled("alice")
		get(s, "alice")
		s.Spec.Running = ptr.To(false)
		Expect(k8sClient.Update(ctx, s)).To(Succeed())
		reconcileUntilSettled("alice")

		sts := &appsv1.StatefulSet{}
		get(sts, "alice")
		Expect(*sts.Spec.Replicas).To(Equal(int32(0)))
		get(s, "alice")
		Expect(s.Status.Phase).To(Equal(gamesv1alpha1.PhaseStopped))
	})

	DescribeTable("honors stop requests when the catalog is broken", func(removeGame bool) {
		s := server("alice", "minecraft")
		s.Namespace = testNamespace
		Expect(k8sClient.Create(ctx, s)).To(Succeed())
		reconcileUntilSettled("alice")
		if removeGame {
			Expect(k8sClient.Delete(ctx, minecraft())).To(Succeed())
		}
		get(s, "alice")
		s.Spec.Size = "removed-size"
		s.Spec.Running = ptr.To(false)
		Expect(k8sClient.Update(ctx, s)).To(Succeed())
		reconcileUntilSettled("alice")
		sts := &appsv1.StatefulSet{}
		get(sts, "alice")
		Expect(*sts.Spec.Replicas).To(BeZero())
		get(s, "alice")
		Expect(s.Status.Phase).To(Equal(gamesv1alpha1.PhaseStopped))
	}, Entry("missing Game", true), Entry("removed size", false))

	It("does not provision resources for a server created stopped", func() {
		s := server("dormant", "missing")
		s.Namespace = testNamespace
		s.Spec.Running = ptr.To(false)
		Expect(k8sClient.Create(ctx, s)).To(Succeed())
		reconcileUntilSettled("dormant")
		get(s, "dormant")
		Expect(s.Status.Phase).To(Equal(gamesv1alpha1.PhaseStopped))
		err := k8sClient.Get(ctx, types.NamespacedName{Name: "dormant-world", Namespace: testNamespace}, &corev1.PersistentVolumeClaim{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("gives each port pool server its own slot, and keeps it", func() {
		for _, name := range []string{"bob", "carol"} {
			s := server(name, "valheim")
			s.Namespace = testNamespace
			Expect(k8sClient.Create(ctx, s)).To(Succeed())
			reconcileUntilSettled(name)
		}

		bob, carol := &gamesv1alpha1.GameServer{}, &gamesv1alpha1.GameServer{}
		get(bob, "bob")
		get(carol, "carol")
		Expect(*bob.Spec.Slot).To(Equal(int32(0)))
		Expect(*carol.Spec.Slot).To(Equal(int32(1)))
		Expect(carol.Status.Address).To(Equal("valheim.example.com:2458"))

		svc := &corev1.Service{}
		get(svc, "carol")
		Expect(svc.Annotations).To(HaveKeyWithValue(tailscaleHostnameKey, "valheim-slot-1"))

		By("refusing to move an assigned slot")
		carol.Spec.Slot = ptr.To[int32](3)
		Expect(k8sClient.Update(ctx, carol)).NotTo(Succeed())
	})

	It("reports an error when every slot is taken", func() {
		game := valheim()
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "valheim"}, game)).To(Succeed())
		for i := range game.Spec.Connection.PortPool.Slots + 1 {
			name := []string{"s0", "s1", "s2", "s3", "s4"}[i]
			s := server(name, "valheim")
			s.Namespace = testNamespace
			Expect(k8sClient.Create(ctx, s)).To(Succeed())
			reconcileUntilSettled(name)
		}
		last := &gamesv1alpha1.GameServer{}
		get(last, "s4")
		Expect(last.Spec.Slot).To(BeNil())
		Expect(last.Status.Phase).To(Equal(gamesv1alpha1.PhaseError))
	})

	It("keeps the world on delete, and removes it only on purge", func() {
		for _, name := range []string{"kept", "purged"} {
			s := server(name, "minecraft")
			s.Namespace = testNamespace
			Expect(k8sClient.Create(ctx, s)).To(Succeed())
			reconcileUntilSettled(name)
		}

		purged := &gamesv1alpha1.GameServer{}
		get(purged, "purged")
		purged.Annotations = map[string]string{gamesv1alpha1.PurgeAnnotation: "true"}
		Expect(k8sClient.Update(ctx, purged)).To(Succeed())

		for _, name := range []string{"kept", "purged"} {
			s := &gamesv1alpha1.GameServer{}
			get(s, name)
			Expect(k8sClient.Delete(ctx, s)).To(Succeed())
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: name, Namespace: testNamespace},
			})
			Expect(err).NotTo(HaveOccurred())
		}

		get(&corev1.PersistentVolumeClaim{}, "kept-world")
		pvc := &corev1.PersistentVolumeClaim{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: "purged-world", Namespace: testNamespace}, pvc)
		// envtest runs no PVC protection controller, so a deleted PVC is gone
		// or, at most, marked for deletion.
		Expect(apierrors.IsNotFound(err) || !pvc.DeletionTimestamp.IsZero()).To(BeTrue())
	})

	It("rejects duplicate and out-of-range restored slots without provisioning them", func() {
		for _, name := range []string{"original", "duplicate", "outside"} {
			s := server(name, "valheim")
			s.Namespace = testNamespace
			s.Spec.Slot = ptr.To[int32](0)
			if name == "outside" {
				s.Spec.Slot = ptr.To[int32](4)
			}
			Expect(k8sClient.Create(ctx, s)).To(Succeed())
			if name == "original" {
				reconcileUntilSettled(name)
				continue
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(s)})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			get(s, name)
			Expect(s.Status.Conditions[0].Reason).To(Equal("SlotConflict"))
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(s), &corev1.Service{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		}
		By("leaving the original holder of the slot running")
		reconcileUntilSettled("original")
		original := &gamesv1alpha1.GameServer{}
		get(original, "original")
		Expect(original.Status.Phase).NotTo(Equal(gamesv1alpha1.PhaseError))
	})

	It("retries an exhausted pool and allocates a subsequently released slot", func() {
		for i, name := range []string{"r0", "r1", "r2", "r3", "waiting"} {
			s := server(name, "valheim")
			s.Namespace = testNamespace
			if i < 4 {
				s.Spec.Slot = ptr.To(int32(i))
			}
			Expect(k8sClient.Create(ctx, s)).To(Succeed())
		}
		request := reconcile.Request{NamespacedName: types.NamespacedName{Name: "waiting", Namespace: testNamespace}}
		result, err := reconciler.Reconcile(ctx, request)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		released := &gamesv1alpha1.GameServer{}
		get(released, "r0")
		Expect(k8sClient.Delete(ctx, released)).To(Succeed())
		reconcileUntilSettled("waiting")
		waiting := &gamesv1alpha1.GameServer{}
		get(waiting, "waiting")
		Expect(*waiting.Spec.Slot).To(BeZero())
	})
})
