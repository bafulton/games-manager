package controller

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	gamesv1alpha1 "github.com/bafulton/games-manager/api/v1alpha1"
)

func valheim() *gameTemplate {
	return &gameTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "valheim"},
		Spec: gamesv1alpha1.GameTemplateSpec{
			Image: "ghcr.io/example/valheim:1.0.0",
			Env:   []corev1.EnvVar{{Name: "WORLD", Value: "Dedicated"}, {Name: "PUBLIC", Value: "false"}},
			Ports: []gamesv1alpha1.GamePort{
				{Name: "game", ContainerPort: 2456, Protocol: corev1.ProtocolUDP},
				{Name: "query", ContainerPort: 2457, Protocol: corev1.ProtocolUDP},
			},
			Connection: gamesv1alpha1.Connection{
				Type: gamesv1alpha1.ConnectionPortPool,
				PortPool: &gamesv1alpha1.PortPoolConnection{
					Slots: 4, PublicHost: "valheim.example.com", BasePort: 2456, Stride: 2,
				},
			},
			Sizes: []gamesv1alpha1.SizePreset{
				{Name: "small", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("3Gi")},
				}},
				{Name: "large", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("6Gi")},
				}},
			},
			DefaultSize:  "small",
			Service:      gamesv1alpha1.GameService{Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: ptr.To("tailscale"), AllocateLoadBalancerNodePorts: ptr.To(false)},
			Integrations: gamesv1alpha1.GameIntegrations{Tailscale: &gamesv1alpha1.TailscaleIntegration{Tags: "tag:k8s-games"}},
			Storage:      gamesv1alpha1.GameStorage{Size: resource.MustParse("10Gi"), MountPath: "/config"},
			Arch:         "amd64",
		},
	}
}

func minecraft() *gameTemplate {
	g := valheim()
	g.Name = "minecraft"
	g.Spec.Ports = []gamesv1alpha1.GamePort{{Name: "java", ContainerPort: 25565}}
	g.Spec.Connection = gamesv1alpha1.Connection{
		Type:     gamesv1alpha1.ConnectionHostname,
		Hostname: &gamesv1alpha1.HostnameConnection{Domain: "mc.example.com"},
	}
	g.Spec.Arch = ""
	g.Spec.Service = gamesv1alpha1.GameService{}
	g.Spec.Integrations = gamesv1alpha1.GameIntegrations{MCRouter: true}
	return g
}

func server(name, game string) *gamesv1alpha1.GameServer {
	return &gamesv1alpha1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "games"},
		Spec:       gamesv1alpha1.GameServerSpec{Game: game, GameVersion: game + "-v1"},
	}
}

func TestAddress(t *testing.T) {
	alice := server("alice", "minecraft")
	if got, want := address(minecraft(), alice), "alice.games.mc.example.com"; got != want {
		t.Errorf("hostname address = %q, want %q", got, want)
	}

	bob := server("bob", "valheim")
	if got := address(valheim(), bob); got != "" {
		t.Errorf("address before a slot is assigned = %q, want empty", got)
	}
	bob.Spec.Slot = ptr.To[int32](3)
	// BasePort + slot*stride = 2456 + 3*2.
	if got, want := address(valheim(), bob), "valheim.example.com:2462"; got != want {
		t.Errorf("port pool address = %q, want %q", got, want)
	}
}

func TestHostnameSeparatesNamespaces(t *testing.T) {
	a := server("alice", "minecraft")
	b := a.DeepCopy()
	b.Namespace = "other"
	if address(minecraft(), a) == address(minecraft(), b) {
		t.Fatal("identically named servers in different namespaces must have different addresses")
	}
}

func TestStatefulSet(t *testing.T) {
	game := valheim()
	s := server("bob", "valheim")
	s.Spec.Size = "large"
	s.Spec.Env = []corev1.EnvVar{{Name: "PUBLIC", Value: "true"}}

	sts := &appsv1.StatefulSet{}
	if err := mutateStatefulSet(sts, game, s); err != nil {
		t.Fatal(err)
	}

	if *sts.Spec.Replicas != 1 {
		t.Errorf("replicas = %d, want 1 for a running server", *sts.Spec.Replicas)
	}
	pod := sts.Spec.Template.Spec
	if got := pod.NodeSelector[corev1.LabelArchStable]; got != "amd64" {
		t.Errorf("arch node selector = %q, want amd64", got)
	}
	c := pod.Containers[0]
	if got := c.Resources.Requests[corev1.ResourceMemory]; got.String() != "6Gi" {
		t.Errorf("memory request = %s, want the large preset's 6Gi", got.String())
	}
	// The server's env comes after the Game's, so its PUBLIC wins.
	if last := c.Env[len(c.Env)-1]; last.Name != "PUBLIC" || last.Value != "true" {
		t.Errorf("last env = %+v, want the server's PUBLIC=true", last)
	}
	if got := pod.Volumes[0].PersistentVolumeClaim.ClaimName; got != "bob-world" {
		t.Errorf("claim = %q, want bob-world", got)
	}
	if !*pod.SecurityContext.RunAsNonRoot {
		t.Error("pod should run as non-root by default")
	}

	s.Spec.Running = ptr.To(false)
	if err := mutateStatefulSet(sts, game, s); err != nil {
		t.Fatal(err)
	}
	if *sts.Spec.Replicas != 0 {
		t.Errorf("replicas = %d, want 0 for a stopped server", *sts.Spec.Replicas)
	}

	s.Spec.Size = "huge"
	if err := mutateStatefulSet(sts, game, s); err == nil {
		t.Error("an unknown size should be an error")
	}
}

func TestRestartRollsThePod(t *testing.T) {
	game, s := valheim(), server("bob", "valheim")
	sts := &appsv1.StatefulSet{}
	_ = mutateStatefulSet(sts, game, s)
	if sts.Spec.Template.Annotations[restartedAtAnnotation] != "" {
		t.Fatalf("no restart requested, but template has annotations %v", sts.Spec.Template.Annotations)
	}
	s.Spec.RestartedAt = &metav1.Time{Time: metav1.Now().Time}
	_ = mutateStatefulSet(sts, game, s)
	if sts.Spec.Template.Annotations[restartedAtAnnotation] == "" {
		t.Error("restartedAt should be stamped on the pod template")
	}
}

func TestPVCIsRetainedAndUsesClusterDefaultStorage(t *testing.T) {
	pvc := desiredPVC(valheim(), server("bob", "valheim"))
	if len(pvc.OwnerReferences) != 0 {
		t.Error("the world PVC must not be owned, or deleting the server would delete it")
	}
	if pvc.Spec.StorageClassName != nil {
		t.Errorf("storage class = %v, want nil for the cluster default", pvc.Spec.StorageClassName)
	}
}

func TestPVCStorageOverrides(t *testing.T) {
	g := valheim()
	g.Spec.Storage.StorageClassName = ptr.To("")
	g.Spec.Storage.Labels = map[string]string{"recurring-job.longhorn.io/source": "enabled"}
	g.Spec.Storage.Annotations = map[string]string{"example.com/backup": "daily"}
	pvc := desiredPVC(g, server("bob", "valheim"))
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "" {
		t.Fatal("explicit empty storage class must remain empty")
	}
	if pvc.Labels["recurring-job.longhorn.io/source"] != "enabled" || pvc.Annotations["example.com/backup"] != "daily" {
		t.Fatal("storage provider metadata was not copied")
	}
}

func TestServiceByConnection(t *testing.T) {
	mc := &corev1.Service{}
	mutateService(mc, minecraft(), server("alice", "minecraft"))
	if mc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("hostname game service type = %s, want ClusterIP", mc.Spec.Type)
	}
	if got := mc.Annotations[mcRouterAnnotation]; got != "alice.games.mc.example.com" {
		t.Errorf("mc-router annotation = %q", got)
	}

	s := server("bob", "valheim")
	s.Spec.Slot = ptr.To[int32](2)
	vh := &corev1.Service{}
	mutateService(vh, valheim(), s)
	if vh.Spec.Type != corev1.ServiceTypeLoadBalancer || *vh.Spec.LoadBalancerClass != "tailscale" {
		t.Errorf("port pool service = %s/%v, want a tailscale LoadBalancer", vh.Spec.Type, vh.Spec.LoadBalancerClass)
	}
	if got := vh.Annotations[tailscaleHostnameKey]; got != "valheim-slot-2" {
		t.Errorf("tailnet hostname = %q, want valheim-slot-2", got)
	}
	if got := vh.Annotations[tailscaleTagsKey]; got != "tag:k8s-games" {
		t.Errorf("tailscale tags = %q, want the tag:k8s-games default", got)
	}
	if len(vh.Spec.Ports) != 2 || vh.Spec.Ports[0].Protocol != corev1.ProtocolUDP {
		t.Errorf("ports = %+v, want Valheim's two UDP ports", vh.Spec.Ports)
	}
}

func TestServicePortTracksContainerPortChanges(t *testing.T) {
	g := minecraft()
	g.Spec.Service.Type = corev1.ServiceTypeNodePort
	svc := &corev1.Service{Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{
		Name: "java", Port: 25565, TargetPort: intstr.FromInt32(25565), NodePort: 31000,
	}}}}
	g.Spec.Ports[0].ContainerPort = 25566
	mutateService(svc, g, server("alice", "minecraft"))
	p := svc.Spec.Ports[0]
	if p.Port != 25566 || p.TargetPort != intstr.FromString("java") || p.NodePort != 31000 {
		t.Fatalf("updated port = %+v; expected the new port, named target and retained NodePort", p)
	}
}

func TestGenericServicesDoNotRequireIntegrations(t *testing.T) {
	for _, kind := range []corev1.ServiceType{corev1.ServiceTypeClusterIP, corev1.ServiceTypeNodePort, corev1.ServiceTypeLoadBalancer} {
		t.Run(string(kind), func(t *testing.T) {
			g := minecraft()
			g.Spec.Integrations = gamesv1alpha1.GameIntegrations{}
			g.Spec.Service = gamesv1alpha1.GameService{Type: kind, Annotations: map[string]string{"example.com/exposure": "public"}}
			svc := &corev1.Service{}
			mutateService(svc, g, server("alice", "minecraft"))
			if svc.Spec.Type != kind || svc.Spec.LoadBalancerClass != nil || svc.Annotations[mcRouterAnnotation] != "" || svc.Annotations[tailscaleHostnameKey] != "" {
				t.Fatalf("unexpected integration or Service type: %+v", svc)
			}
			if svc.Annotations["example.com/exposure"] != "public" {
				t.Fatal("custom annotations missing")
			}
			g.Spec.Service.Annotations = nil
			svc.Annotations["external.example.com/keep"] = "yes"
			mutateService(svc, g, server("alice", "minecraft"))
			if svc.Annotations["example.com/exposure"] != "" || svc.Annotations["external.example.com/keep"] != "yes" {
				t.Fatal("annotation ownership not respected")
			}
		})
	}
}

func TestDirectServiceAddresses(t *testing.T) {
	g := minecraft()
	g.Spec.Connection = gamesv1alpha1.Connection{Type: gamesv1alpha1.ConnectionService}
	s := server("alice", "minecraft")
	svc := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Ports: []corev1.ServicePort{{Port: 25565}}}}
	if got := serviceAddress(g, s, svc); got != "alice.games.svc:25565" {
		t.Fatalf("ClusterIP address: %s", got)
	}
	svc.Spec.Type = corev1.ServiceTypeLoadBalancer
	if got := serviceAddress(g, s, svc); got != "" {
		t.Fatalf("pending ingress should have no address: %s", got)
	}
	svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "2001:db8::1"}}
	if got := serviceAddress(g, s, svc); got != "[2001:db8::1]:25565" {
		t.Fatalf("IPv6 ingress: %s", got)
	}
	svc.Spec.Type = corev1.ServiceTypeNodePort
	svc.Spec.Ports[0].NodePort = 31000
	g.Spec.Connection.Service = &gamesv1alpha1.ServiceConnection{Host: "node.example.com"}
	if got := serviceAddress(g, s, svc); got != "node.example.com:31000" {
		t.Fatalf("NodePort address: %s", got)
	}
}
