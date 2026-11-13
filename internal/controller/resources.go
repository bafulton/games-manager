package controller

import (
	"fmt"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	gamesv1alpha1 "github.com/bafulton/games-manager/api/v1alpha1"
)

const (
	labelGame   = "games-manager.io/game"
	labelServer = "games-manager.io/server"

	// restartedAtAnnotation goes on the pod template, so changing
	// spec.restartedAt rolls the pod.
	restartedAtAnnotation = "games-manager.io/restartedAt"

	// managedAnnotationsKey lists the Service annotations that came from
	// the Game, so the controller removes one the Game stops setting while
	// leaving annotations other controllers add alone.
	managedAnnotationsKey = "games-manager.io/managed-annotations"

	gameContainerName = "game"
	worldVolumeName   = "world"
)

func labels(game *gamesv1alpha1.Game, server *gamesv1alpha1.GameServer) map[string]string {
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

// sizeFor returns the server's size preset, falling back to the Game's
// default.
func sizeFor(game *gamesv1alpha1.Game, server *gamesv1alpha1.GameServer) (*gamesv1alpha1.SizePreset, error) {
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

// versionFor returns the server's version, or nil for a Game without
// versions. The controller records the default in the server's spec before
// building anything, so the server names its version by then.
func versionFor(game *gamesv1alpha1.Game, server *gamesv1alpha1.GameServer) (*gamesv1alpha1.GameVersion, error) {
	if len(game.Spec.Versions) == 0 {
		if server.Spec.Version != "" {
			return nil, fmt.Errorf("game %q has no versions, but the server names %q", game.Name, server.Spec.Version)
		}
		return nil, nil
	}
	for i := range game.Spec.Versions {
		if game.Spec.Versions[i].Name == server.Spec.Version {
			return &game.Spec.Versions[i], nil
		}
	}
	return nil, fmt.Errorf("game %q has no version %q", game.Name, server.Spec.Version)
}

// address is where players connect, or "" while a PortPool server has no
// slot yet. It's "" for a Direct connection, whose address comes from the
// Service: see serviceAddress.
func address(game *gamesv1alpha1.Game, server *gamesv1alpha1.GameServer) string {
	conn := game.Spec.Connection
	switch conn.Type {
	case gamesv1alpha1.ConnectionHostname:
		return fmt.Sprintf("%s.%s", server.Name, conn.Hostname.Domain)
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

// serviceAddress is where players connect to a Direct server: the host,
// and the port of the Game's first port, or "" until a load balancer has
// an address.
func serviceAddress(game *gamesv1alpha1.Game, server *gamesv1alpha1.GameServer, svc *corev1.Service) string {
	if len(svc.Spec.Ports) == 0 {
		return ""
	}
	port := svc.Spec.Ports[0].Port
	if svc.Spec.Type == corev1.ServiceTypeNodePort {
		port = svc.Spec.Ports[0].NodePort
	}

	host := ""
	if direct := game.Spec.Connection.Direct; direct != nil && direct.Host != "" {
		host = expand(direct.Host, game, server)
	} else {
		switch svc.Spec.Type {
		case corev1.ServiceTypeLoadBalancer:
			for _, ingress := range svc.Status.LoadBalancer.Ingress {
				host = ingress.IP
				if host == "" {
					host = ingress.Hostname
				}
				if host != "" {
					break
				}
			}
		case corev1.ServiceTypeClusterIP:
			host = fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace)
		}
	}
	if host == "" || port == 0 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(int(port)))
}

func desiredPVC(game *gamesv1alpha1.Game, server *gamesv1alpha1.GameServer) *corev1.PersistentVolumeClaim {
	// The selector labels go last, so a Game can't override them.
	l := maps.Clone(game.Spec.Storage.Labels)
	if l == nil {
		l = map[string]string{}
	}
	maps.Copy(l, labels(game, server))

	// No owner reference: deleting the GameServer must not delete its
	// world. The finalizer deletes it only when the server is purged.
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pvcName(server),
			Namespace:   server.Namespace,
			Labels:      l,
			Annotations: maps.Clone(game.Spec.Storage.Annotations),
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
func mutateStatefulSet(sts *appsv1.StatefulSet, game *gamesv1alpha1.Game, server *gamesv1alpha1.GameServer) error {
	size, err := sizeFor(game, server)
	if err != nil {
		return err
	}
	version, err := versionFor(game, server)
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
	tpl.Annotations = nil
	if server.Spec.RestartedAt != nil {
		tpl.Annotations = map[string]string{
			restartedAtAnnotation: server.Spec.RestartedAt.UTC().Format("2006-01-02T15:04:05Z"),
		}
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
	pod.NodeSelector = nodeSelector(game)
	pod.Affinity = game.Spec.Affinity
	pod.Tolerations = game.Spec.Tolerations
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

	// Later entries win, so a version overrides the Game's env, and the
	// server overrides both.
	env := append([]corev1.EnvVar{}, game.Spec.Env...)
	image, args := game.Spec.Image, game.Spec.Args
	if version != nil {
		env = append(env, version.Env...)
		if version.Image != "" {
			image = version.Image
		}
		if version.Args != nil {
			args = version.Args
		}
	}
	env = append(env, server.Spec.Env...)

	container := corev1.Container{
		Name:           gameContainerName,
		Image:          image,
		Command:        game.Spec.Command,
		Args:           args,
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

// nodeSelector is the Game's nodeSelector with its arch added, or nil when
// there is neither.
func nodeSelector(game *gamesv1alpha1.Game) map[string]string {
	if len(game.Spec.NodeSelector) == 0 && game.Spec.Arch == "" {
		return nil
	}
	selector := make(map[string]string, len(game.Spec.NodeSelector)+1)
	for k, v := range game.Spec.NodeSelector {
		selector[k] = v
	}
	if game.Spec.Arch != "" {
		selector[corev1.LabelArchStable] = game.Spec.Arch
	}
	return selector
}

func protocol(p gamesv1alpha1.GamePort) corev1.Protocol {
	if p.Protocol == "" {
		return corev1.ProtocolTCP
	}
	return p.Protocol
}

// expand replaces the $(VARIABLES) a Game's service annotations and labels
// can use with the server's values.
func expand(value string, game *gamesv1alpha1.Game, server *gamesv1alpha1.GameServer) string {
	slot := ""
	if server.Spec.Slot != nil {
		slot = strconv.Itoa(int(*server.Spec.Slot))
	}
	return strings.NewReplacer(
		"$(SERVER)", server.Name,
		"$(NAMESPACE)", server.Namespace,
		"$(GAME)", game.Name,
		"$(SLOT)", slot,
		"$(ADDRESS)", address(game, server),
	).Replace(value)
}

// mutateService sets the fields of svc the controller owns, from the Game's
// service settings.
func mutateService(svc *corev1.Service, game *gamesv1alpha1.Game, server *gamesv1alpha1.GameServer) {
	spec := game.Spec.Service

	svc.Labels = map[string]string{}
	for k, v := range spec.Labels {
		svc.Labels[k] = expand(v, game, server)
	}
	maps.Copy(svc.Labels, labels(game, server))

	if svc.Annotations == nil {
		svc.Annotations = map[string]string{}
	}
	for _, k := range strings.Split(svc.Annotations[managedAnnotationsKey], ",") {
		delete(svc.Annotations, k)
	}
	delete(svc.Annotations, managedAnnotationsKey)
	if len(spec.Annotations) > 0 {
		for k, v := range spec.Annotations {
			svc.Annotations[k] = expand(v, game, server)
		}
		svc.Annotations[managedAnnotationsKey] = strings.Join(slices.Sorted(maps.Keys(spec.Annotations)), ",")
	}

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

	svc.Spec.Type = spec.Type
	if svc.Spec.Type == "" {
		svc.Spec.Type = corev1.ServiceTypeClusterIP
	}
	svc.Spec.LoadBalancerClass = spec.LoadBalancerClass
	svc.Spec.AllocateLoadBalancerNodePorts = spec.AllocateLoadBalancerNodePorts
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer && spec.AllocateLoadBalancerNodePorts == nil {
		// The API server defaults it to true; keep that, or every update
		// would look like a change.
		svc.Spec.AllocateLoadBalancerNodePorts = ptr.To(true)
	}
	if svc.Spec.Type == corev1.ServiceTypeClusterIP {
		// Node ports don't exist on a ClusterIP Service.
		for i := range svc.Spec.Ports {
			svc.Spec.Ports[i].NodePort = 0
		}
	}
}
