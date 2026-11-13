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

func valheim() *gamesv1alpha1.Game {
	return &gamesv1alpha1.Game{
		ObjectMeta: metav1.ObjectMeta{Name: "valheim"},
		Spec: gamesv1alpha1.GameSpec{
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
			DefaultSize: "small",
			Storage:     gamesv1alpha1.GameStorage{Size: resource.MustParse("10Gi"), MountPath: "/config"},
			Arch:        "amd64",
		},
	}
}

func minecraft() *gamesv1alpha1.Game {
	g := valheim()
	g.Name = "minecraft"
	g.Spec.Ports = []gamesv1alpha1.GamePort{{Name: "java", ContainerPort: 25565}}
	g.Spec.Connection = gamesv1alpha1.Connection{
		Type:     gamesv1alpha1.ConnectionHostname,
		Hostname: &gamesv1alpha1.HostnameConnection{Domain: "mc.example.com"},
	}
	g.Spec.Arch = ""
	return g
}

func server(name, game string) *gamesv1alpha1.GameServer {
	return &gamesv1alpha1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "games"},
		Spec:       gamesv1alpha1.GameServerSpec{Game: game},
	}
}

func TestAddress(t *testing.T) {
	alice := server("alice", "minecraft")
	if got, want := address(minecraft(), alice), "alice.mc.example.com"; got != want {
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

func TestScheduling(t *testing.T) {
	game, s := valheim(), server("bob", "valheim")
	game.Spec.NodeSelector = map[string]string{"example.com/zone": "home"}
	game.Spec.Tolerations = []corev1.Toleration{{Key: "example.com/busy", Effect: corev1.TaintEffectPreferNoSchedule}}
	game.Spec.Affinity = &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
		PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
			Weight: 50,
			Preference: corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key: "example.com/zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"cloud"},
			}}},
		}},
	}}

	sts := &appsv1.StatefulSet{}
	if err := mutateStatefulSet(sts, game, s); err != nil {
		t.Fatal(err)
	}
	pod := sts.Spec.Template.Spec
	want := map[string]string{"example.com/zone": "home", corev1.LabelArchStable: "amd64"}
	if len(pod.NodeSelector) != len(want) {
		t.Errorf("node selector = %v, want %v", pod.NodeSelector, want)
	}
	for k, v := range want {
		if pod.NodeSelector[k] != v {
			t.Errorf("node selector[%q] = %q, want %q", k, pod.NodeSelector[k], v)
		}
	}
	if game.Spec.NodeSelector[corev1.LabelArchStable] != "" {
		t.Error("adding arch must not modify the Game's own nodeSelector")
	}
	if pod.Affinity != game.Spec.Affinity || len(pod.Tolerations) != 1 {
		t.Errorf("affinity and tolerations should pass through: %+v, %+v", pod.Affinity, pod.Tolerations)
	}

	game.Spec.NodeSelector, game.Spec.Arch = nil, ""
	if err := mutateStatefulSet(sts, game, s); err != nil {
		t.Fatal(err)
	}
	if sts.Spec.Template.Spec.NodeSelector != nil {
		t.Errorf("with no arch or nodeSelector, node selector = %v, want none", sts.Spec.Template.Spec.NodeSelector)
	}
}

func TestVersions(t *testing.T) {
	game, s := minecraft(), server("alice", "minecraft")
	game.Spec.Args = []string{"--nogui"}
	game.Spec.Versions = []gamesv1alpha1.GameVersion{
		{Name: "1.21", Env: []corev1.EnvVar{{Name: "VERSION", Value: "1.21.4"}}},
		{Name: "1.16", Image: "ghcr.io/example/minecraft:java8", Args: []string{}, Env: []corev1.EnvVar{
			{Name: "VERSION", Value: "1.16.5"}, {Name: "PUBLIC", Value: "true"},
		}},
	}
	game.Spec.DefaultVersion = "1.21"

	s.Spec.Version = "1.21"
	sts := &appsv1.StatefulSet{}
	if err := mutateStatefulSet(sts, game, s); err != nil {
		t.Fatal(err)
	}
	c := sts.Spec.Template.Spec.Containers[0]
	if c.Image != game.Spec.Image || len(c.Args) != 1 {
		t.Errorf("image, args = %s, %v; a version without overrides should keep the Game's", c.Image, c.Args)
	}
	if last := c.Env[len(c.Env)-1]; last.Name != "VERSION" || last.Value != "1.21.4" {
		t.Errorf("last env = %+v, want the version's VERSION after the Game's env", last)
	}

	s.Spec.Version = "1.16"
	s.Spec.Env = []corev1.EnvVar{{Name: "PUBLIC", Value: "false"}}
	if err := mutateStatefulSet(sts, game, s); err != nil {
		t.Fatal(err)
	}
	c = sts.Spec.Template.Spec.Containers[0]
	if c.Image != "ghcr.io/example/minecraft:java8" || len(c.Args) != 0 {
		t.Errorf("image, args = %s, %v; want the version's overrides", c.Image, c.Args)
	}
	// The server's env comes after the version's, so its PUBLIC wins.
	if last := c.Env[len(c.Env)-1]; last.Name != "PUBLIC" || last.Value != "false" {
		t.Errorf("last env = %+v, want the server's PUBLIC=false", last)
	}

	s.Spec.Version = "1.12"
	if err := mutateStatefulSet(sts, game, s); err == nil {
		t.Error("an unknown version should be an error")
	}
	game.Spec.Versions, game.Spec.DefaultVersion = nil, ""
	if err := mutateStatefulSet(sts, game, s); err == nil {
		t.Error("naming a version of a Game without versions should be an error")
	}
}

func TestRestartRollsThePod(t *testing.T) {
	game, s := valheim(), server("bob", "valheim")
	sts := &appsv1.StatefulSet{}
	_ = mutateStatefulSet(sts, game, s)
	if len(sts.Spec.Template.Annotations) != 0 {
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
	if got := mc.Annotations[mcRouterAnnotation]; got != "alice.mc.example.com" {
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
