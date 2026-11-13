package controller

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	gamesv1alpha1 "github.com/bafulton/games-manager/api/v1alpha1"
)

// gameTemplate is the resolved catalog identity and selected immutable recipe.
type gameTemplate struct {
	metav1.ObjectMeta
	Spec gamesv1alpha1.GameTemplateSpec
}

const (
	labelGame   = "games.homelab.io/game"
	labelServer = "games.homelab.io/server"

	// restartedAtAnnotation goes on the pod template, so changing
	// spec.restartedAt rolls the pod.
	restartedAtAnnotation = "games.homelab.io/restartedAt"

	mcRouterAnnotation   = "mc-router.itzg.me/externalServerName"
	tailscaleHostnameKey = "tailscale.com/hostname"
	tailscaleTagsKey     = "tailscale.com/tags"
	gameContainerName    = "game"
	worldVolumeName      = "world"
)

func labels(game *gameTemplate, server *gamesv1alpha1.GameServer) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       game.Name,
		"app.kubernetes.io/instance":   server.Name,
		"app.kubernetes.io/managed-by": "games-manager",
		labelGame:                      game.Name,
		labelServer:                    server.Name,
	}
}

func selectorLabels(server *gamesv1alpha1.GameServer) map[string]string {
	return map[string]string{labelServer: server.Name}
}

// pvcName is derived from the server's name alone, so recreating a
// GameServer from a backup finds its world again.
func pvcName(server *gamesv1alpha1.GameServer) string {
	return server.Name + "-world"
}

// slotHostname is the tailnet name of a PortPool slot's Service. The edge
// forwarder routes each slot's public ports to it.
func slotHostname(game *gameTemplate, slot int32) string {
	return fmt.Sprintf("%s-slot-%d", game.Name, slot)
}

// sizeFor returns the server's size preset, falling back to the Game's
// default.
func sizeFor(game *gameTemplate, server *gamesv1alpha1.GameServer) (*gamesv1alpha1.SizePreset, error) {
	name := server.Spec.Size
	if name == "" {
		name = game.Spec.DefaultSize
	}
	for i := range game.Spec.Sizes {
		if game.Spec.Sizes[i].Name == name {
			return &game.Spec.Sizes[i], nil
		}
	}
	return nil, fmt.Errorf("game %q has no size %q", game.Name, name)
}

// address is where players connect, or "" while a PortPool server has no
// slot yet.
func address(game *gameTemplate, server *gamesv1alpha1.GameServer) string {
	conn := game.Spec.Connection
	switch conn.Type {
	case gamesv1alpha1.ConnectionHostname:
		return fmt.Sprintf("%s.%s.%s", server.Name, server.Namespace, conn.Hostname.Domain)
	case gamesv1alpha1.ConnectionPortPool:
		if server.Spec.Slot == nil {
			return ""
		}
		pool := conn.PortPool
		stride := pool.Stride
		if stride == 0 {
			stride = 1
		}
		return fmt.Sprintf("%s:%d", pool.PublicHost, pool.BasePort+*server.Spec.Slot*stride)
	}
	return ""
}

func desiredPVC(game *gameTemplate, server *gamesv1alpha1.GameServer) *corev1.PersistentVolumeClaim {
	l := map[string]string{}
	for key, value := range game.Spec.Storage.Labels {
		l[key] = value
	}
	for key, value := range labels(game, server) {
		l[key] = value
	}

	// No owner reference: deleting the GameServer must not delete its
	// world. The finalizer deletes it only when the server is purged.
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pvcName(server),
			Namespace:   server.Namespace,
			Labels:      l,
			Annotations: game.Spec.Storage.Annotations,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: game.Spec.Storage.StorageClassName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: game.Spec.Storage.Size},
			},
		},
	}
}

func defaultPodSecurityContext() *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		RunAsNonRoot:        ptr.To(true),
		RunAsUser:           ptr.To[int64](1000),
		RunAsGroup:          ptr.To[int64](1000),
		FSGroup:             ptr.To[int64](1000),
		FSGroupChangePolicy: ptr.To(corev1.FSGroupChangeOnRootMismatch),
		SeccompProfile:      &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}

// mutateStatefulSet sets the fields of sts that the controller owns, leaving
// defaulted and server-managed fields alone so CreateOrUpdate only writes on
// a real change.
func mutateStatefulSet(sts *appsv1.StatefulSet, game *gameTemplate, server *gamesv1alpha1.GameServer) error {
	size, err := sizeFor(game, server)
	if err != nil {
		return err
	}

	replicas := int32(0)
	if server.IsRunning() {
		replicas = 1
	}

	sts.Labels = labels(game, server)
	sts.Spec.Replicas = ptr.To(replicas)
	sts.Spec.ServiceName = server.Name
	// Set once at creation; the selector is immutable.
	if sts.Spec.Selector == nil {
		sts.Spec.Selector = &metav1.LabelSelector{MatchLabels: selectorLabels(server)}
	}

	tpl := &sts.Spec.Template
	tpl.Labels = labels(game, server)
	tpl.Annotations = map[string]string{"games.homelab.io/gameVersion": server.Spec.GameVersion}
	if server.Spec.RestartedAt != nil {
		tpl.Annotations[restartedAtAnnotation] = server.Spec.RestartedAt.UTC().Format("2006-01-02T15:04:05Z")
	}

	pod := &tpl.Spec
	pod.TerminationGracePeriodSeconds = game.Spec.TerminationGracePeriodSeconds
	if pod.TerminationGracePeriodSeconds == nil {
		pod.TerminationGracePeriodSeconds = ptr.To[int64](120)
	}
	pod.SecurityContext = game.Spec.PodSecurityContext
	if pod.SecurityContext == nil {
		pod.SecurityContext = defaultPodSecurityContext()
	}
	pod.NodeSelector = nil
	if game.Spec.Arch != "" {
		pod.NodeSelector = map[string]string{corev1.LabelArchStable: game.Spec.Arch}
	}
	pod.Volumes = []corev1.Volume{{
		Name: worldVolumeName,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName(server)},
		},
	}}

	ports := make([]corev1.ContainerPort, 0, len(game.Spec.Ports))
	for _, p := range game.Spec.Ports {
		ports = append(ports, corev1.ContainerPort{
			Name:          p.Name,
			ContainerPort: p.ContainerPort,
			Protocol:      protocol(p),
		})
	}

	env := append(append([]corev1.EnvVar{}, game.Spec.Env...), server.Spec.Env...)

	container := corev1.Container{
		Name:           gameContainerName,
		Image:          game.Spec.Image,
		Command:        game.Spec.Command,
		Args:           game.Spec.Args,
		Env:            env,
		Ports:          ports,
		Resources:      size.Resources,
		ReadinessProbe: game.Spec.ReadinessProbe,
		Lifecycle:      game.Spec.Lifecycle,
		VolumeMounts: []corev1.VolumeMount{{
			Name:      worldVolumeName,
			MountPath: game.Spec.Storage.MountPath,
		}},
		SecurityContext: game.Spec.SecurityContext,
	}
	if container.SecurityContext == nil {
		container.SecurityContext = &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		}
	}
	// Keep fields the API server defaulted on the existing container, so an
	// unchanged spec doesn't look changed.
	if len(pod.Containers) == 1 {
		existing := pod.Containers[0]
		container.ImagePullPolicy = existing.ImagePullPolicy
		container.TerminationMessagePath = existing.TerminationMessagePath
		container.TerminationMessagePolicy = existing.TerminationMessagePolicy
	}
	pod.Containers = []corev1.Container{container}
	return nil
}

func protocol(p gamesv1alpha1.GamePort) corev1.Protocol {
	if p.Protocol == "" {
		return corev1.ProtocolTCP
	}
	return p.Protocol
}

// mutateService sets the fields of svc the controller owns. Hostname games
// get a ClusterIP that mc-router discovers by annotation; PortPool games get
// a Tailscale LoadBalancer named for their slot, which the edge forwards to.
func mutateService(svc *corev1.Service, game *gameTemplate, server *gamesv1alpha1.GameServer) {
	svc.Labels = labels(game, server)
	if svc.Annotations == nil {
		svc.Annotations = map[string]string{}
	}
	delete(svc.Annotations, mcRouterAnnotation)
	delete(svc.Annotations, tailscaleHostnameKey)
	delete(svc.Annotations, tailscaleTagsKey)

	svc.Spec.Selector = selectorLabels(server)

	ports := make([]corev1.ServicePort, 0, len(game.Spec.Ports))
	for _, p := range game.Spec.Ports {
		ports = append(ports, corev1.ServicePort{
			Name:       p.Name,
			Port:       p.ContainerPort,
			TargetPort: intstr.FromString(p.Name),
			Protocol:   protocol(p),
		})
	}
	// Keep allocated or defaulted fields of existing ports.
	for i := range ports {
		for _, old := range svc.Spec.Ports {
			if old.Name == ports[i].Name {
				ports[i].NodePort = old.NodePort
			}
		}
	}
	svc.Spec.Ports = ports

	switch game.Spec.Connection.Type {
	case gamesv1alpha1.ConnectionHostname:
		svc.Annotations[mcRouterAnnotation] = address(game, server)
		svc.Spec.Type = corev1.ServiceTypeClusterIP
	case gamesv1alpha1.ConnectionPortPool:
		pool := game.Spec.Connection.PortPool
		tags := pool.TailscaleTags
		if tags == "" {
			tags = "tag:k8s-games"
		}
		if server.Spec.Slot != nil {
			svc.Annotations[tailscaleHostnameKey] = slotHostname(game, *server.Spec.Slot)
		}
		svc.Annotations[tailscaleTagsKey] = tags
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
		svc.Spec.LoadBalancerClass = ptr.To("tailscale")
		svc.Spec.AllocateLoadBalancerNodePorts = ptr.To(false)
	}
}
