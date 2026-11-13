package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gamesv1alpha1 "github.com/bafulton/games-manager/api/v1alpha1"
)

const finalizer = "games.homelab.io/finalizer"

// GameServerReconciler turns each GameServer into a StatefulSet, a PVC and a
// Service, using its Game as the template.
type GameServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// APIReader reads straight from the API server. Slot allocation uses it,
	// because the cache can lag a slot just assigned to another server.
	APIReader client.Reader
}

// +kubebuilder:rbac:groups=games.homelab.io,resources=games,verbs=get;list;watch
// +kubebuilder:rbac:groups=games.homelab.io,resources=gameservers,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=games.homelab.io,resources=gameservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=games.homelab.io,resources=gameservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;delete

func (r *GameServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	server := &gamesv1alpha1.GameServer{}
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !server.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.finalize(ctx, server)
	}
	if controllerutil.AddFinalizer(server, finalizer) {
		if err := r.Update(ctx, server); err != nil {
			return ctrl.Result{}, err
		}
	}
	// Always honor a stop request, even if its Game or size no longer exists.
	// Do not rebuild resources here: stopping must also work during a broken
	// catalog update, without allocating a slot or creating a world.
	if !server.IsRunning() {
		sts := &appsv1.StatefulSet{}
		err := r.Get(ctx, req.NamespacedName, sts)
		if err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil {
			if !metav1.IsControlledBy(sts, server) {
				return ctrl.Result{}, r.setError(ctx, server, "ResourceConflict", "the StatefulSet is not owned by this server")
			}
			if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 0 {
				sts.Spec.Replicas = ptr.To[int32](0)
				if err := r.Update(ctx, sts); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
		return ctrl.Result{}, r.updateStatus(ctx, nil, server, sts)
	}

	game := &gamesv1alpha1.Game{}
	if err := r.Get(ctx, types.NamespacedName{Name: server.Spec.Game}, game); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, r.setError(ctx, server, "GameNotFound", fmt.Sprintf("no Game named %q", server.Spec.Game))
		}
		return ctrl.Result{}, err
	}
	if server.Spec.Version == "" && game.Spec.DefaultVersion != "" {
		// Record the default, so a later change to it doesn't move this
		// server. A server created stopped gets it on its first start: it
		// has no world yet that an older version would need.
		server.Spec.Version = game.Spec.DefaultVersion
		log.Info("assigned version", "version", server.Spec.Version)
		// The update triggers the next reconcile, which builds the resources.
		return ctrl.Result{}, r.Update(ctx, server)
	}
	if _, err := sizeFor(game, server); err != nil {
		return ctrl.Result{}, r.setError(ctx, server, "UnknownSize", err.Error())
	}
	if _, err := versionFor(game, server); err != nil {
		return ctrl.Result{}, r.setError(ctx, server, "UnknownVersion", err.Error())
	}

	if game.Spec.Connection.Type == gamesv1alpha1.ConnectionPortPool && server.Spec.Slot == nil {
		slot, err := r.freeSlot(ctx, game)
		if err != nil {
			return ctrl.Result{}, err
		}
		if slot == nil {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, r.setError(ctx, server, "NoSlotAvailable",
				fmt.Sprintf("all %d slots of Game %q are taken", game.Spec.Connection.PortPool.Slots, game.Name))
		}
		server.Spec.Slot = slot
		log.Info("assigned slot", "slot", *slot)
		// The update triggers the next reconcile, which builds the resources.
		return ctrl.Result{}, r.Update(ctx, server)
	}
	if game.Spec.Connection.Type == gamesv1alpha1.ConnectionPortPool {
		if err := r.validateSlot(ctx, game, server); err != nil {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, r.setError(ctx, server, "SlotConflict", err.Error())
		}
	}

	if game.Spec.Connection.Type == gamesv1alpha1.ConnectionHostname {
		conflict, err := r.hostnameConflict(ctx, game, server)
		if err != nil {
			return ctrl.Result{}, err
		}
		if conflict != "" {
			return ctrl.Result{RequeueAfter: 10 * time.Second}, r.setError(ctx, server, "HostnameConflict", conflict)
		}
	}

	if err := r.ensurePVC(ctx, game, server); err != nil {
		return ctrl.Result{}, err
	}

	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: server.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		if err := mutateStatefulSet(sts, game, server); err != nil {
			return err
		}
		return controllerutil.SetControllerReference(server, sts, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, err
	}

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: server.Namespace}}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		mutateService(svc, game, server)
		return controllerutil.SetControllerReference(server, svc, r.Scheme)
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, r.updateStatus(ctx, game, server, sts)
}

// validateSlot checks explicit/restored reservations too. When two servers
// hold the same slot, the older claim keeps it and the newer reports the
// conflict.
// A conflicting slot is never silently changed, because that would change
// the public address.
func (r *GameServerReconciler) validateSlot(ctx context.Context, game *gamesv1alpha1.Game, server *gamesv1alpha1.GameServer) error {
	slot := *server.Spec.Slot
	if slot < 0 || slot >= game.Spec.Connection.PortPool.Slots {
		return fmt.Errorf("slot %d is outside Game %q's pool", slot, game.Name)
	}
	servers := &gamesv1alpha1.GameServerList{}
	if err := r.APIReader.List(ctx, servers); err != nil {
		return err
	}
	for _, other := range servers.Items {
		if other.UID != server.UID && other.Spec.Game == game.Name && other.Spec.Slot != nil && *other.Spec.Slot == slot &&
			olderThan(&other, server) {
			return fmt.Errorf("slot %d is also reserved by %s/%s", slot, other.Namespace, other.Name)
		}
	}
	return nil
}

// hostnameConflict describes the server in another namespace that already
// claims server's hostname, or returns "" when there is none. Hostnames
// leave out the namespace, so same-named servers in two namespaces of
// Games sharing a domain would collide; the older claim keeps the name.
func (r *GameServerReconciler) hostnameConflict(ctx context.Context, game *gamesv1alpha1.Game, server *gamesv1alpha1.GameServer) (string, error) {
	servers := &gamesv1alpha1.GameServerList{}
	if err := r.APIReader.List(ctx, servers); err != nil {
		return "", err
	}
	want := address(game, server)
	for _, other := range servers.Items {
		if other.Name != server.Name || other.Namespace == server.Namespace || !olderThan(&other, server) {
			continue
		}
		otherGame := game
		if other.Spec.Game != game.Name {
			otherGame = &gamesv1alpha1.Game{}
			if err := r.Get(ctx, types.NamespacedName{Name: other.Spec.Game}, otherGame); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return "", err
			}
			if otherGame.Spec.Connection.Type != gamesv1alpha1.ConnectionHostname {
				continue
			}
		}
		if address(otherGame, &other) == want {
			return fmt.Sprintf("hostname %s is already used by %s/%s", want, other.Namespace, other.Name), nil
		}
	}
	return "", nil
}

// olderThan reports whether a's claim predates b's: a server already
// provisioned (it reports an address), running or stopped, beats one that
// isn't, then the earlier created wins. Creation times have one-second
// precision, so namespace and name break ties, making exactly one of two
// servers the winner.
func olderThan(a, b *gamesv1alpha1.GameServer) bool {
	if provisioned := a.Status.Address != ""; provisioned != (b.Status.Address != "") {
		return provisioned
	}
	if !a.CreationTimestamp.Equal(&b.CreationTimestamp) {
		return a.CreationTimestamp.Before(&b.CreationTimestamp)
	}
	return a.Namespace+"/"+a.Name < b.Namespace+"/"+b.Name
}

// ensurePVC creates the server's world volume if it doesn't exist. An
// existing one, including one left by a deleted server of the same name, is
// reused as it is.
func (r *GameServerReconciler) ensurePVC(ctx context.Context, game *gamesv1alpha1.Game, server *gamesv1alpha1.GameServer) error {
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: pvcName(server), Namespace: server.Namespace}, pvc)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	return r.Create(ctx, desiredPVC(game, server))
}

// freeSlot returns the lowest slot of game's pool that no GameServer holds,
// or nil when every slot is taken.
func (r *GameServerReconciler) freeSlot(ctx context.Context, game *gamesv1alpha1.Game) (*int32, error) {
	servers := &gamesv1alpha1.GameServerList{}
	if err := r.APIReader.List(ctx, servers); err != nil {
		return nil, err
	}
	taken := map[int32]bool{}
	for _, s := range servers.Items {
		if s.Spec.Game == game.Name && s.Spec.Slot != nil {
			taken[*s.Spec.Slot] = true
		}
	}
	for slot := int32(0); slot < game.Spec.Connection.PortPool.Slots; slot++ {
		if !taken[slot] {
			return &slot, nil
		}
	}
	return nil, nil
}

// finalize deletes the server's PVC if the server is being purged, then lets
// the deletion finish. The StatefulSet and Service go with the GameServer
// through their owner references.
func (r *GameServerReconciler) finalize(ctx context.Context, server *gamesv1alpha1.GameServer) error {
	if !controllerutil.ContainsFinalizer(server, finalizer) {
		return nil
	}
	if server.Annotations[gamesv1alpha1.PurgeAnnotation] == "true" {
		pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: pvcName(server), Namespace: server.Namespace}}
		if err := r.Delete(ctx, pvc); client.IgnoreNotFound(err) != nil {
			return err
		}
		logf.FromContext(ctx).Info("purged world", "pvc", pvc.Name)
	}
	controllerutil.RemoveFinalizer(server, finalizer)
	return r.Update(ctx, server)
}

func (r *GameServerReconciler) updateStatus(ctx context.Context, game *gamesv1alpha1.Game, server *gamesv1alpha1.GameServer, sts *appsv1.StatefulSet) error {
	ready := sts.Status.ReadyReplicas >= 1
	phase := gamesv1alpha1.PhaseStarting
	reason, message := "Starting", "waiting for the server's pod to be ready"
	switch {
	case !server.IsRunning():
		phase, reason, message = gamesv1alpha1.PhaseStopped, "Stopped", "the server is stopped"
	case ready:
		phase, reason, message = gamesv1alpha1.PhaseRunning, "Running", "the server is ready for players"
	}

	status := metav1.ConditionFalse
	if ready && server.IsRunning() {
		status = metav1.ConditionTrue
	}

	server.Status.Phase = phase
	if game != nil {
		server.Status.Address = address(game, server)
	}
	server.Status.ObservedGeneration = server.Generation
	meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
		Type:               gamesv1alpha1.ConditionReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: server.Generation,
	})
	return r.Status().Update(ctx, server)
}

func (r *GameServerReconciler) setError(ctx context.Context, server *gamesv1alpha1.GameServer, reason, message string) error {
	server.Status.Phase = gamesv1alpha1.PhaseError
	server.Status.ObservedGeneration = server.Generation
	meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
		Type:               gamesv1alpha1.ConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: server.Generation,
	})
	return r.Status().Update(ctx, server)
}

// serversOfGame maps a Game to all of its GameServers, so changing a Game
// updates every server built from it.
func (r *GameServerReconciler) serversOfGame(ctx context.Context, obj client.Object) []reconcile.Request {
	servers := &gamesv1alpha1.GameServerList{}
	if err := r.List(ctx, servers); err != nil {
		logf.FromContext(ctx).Error(err, "listing GameServers for Game", "game", obj.GetName())
		return nil
	}
	var requests []reconcile.Request
	for _, s := range servers.Items {
		if s.Spec.Game == obj.GetName() {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&s)})
		}
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *GameServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.APIReader == nil {
		r.APIReader = mgr.GetAPIReader()
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&gamesv1alpha1.GameServer{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Watches(&gamesv1alpha1.Game{}, handler.EnqueueRequestsFromMapFunc(r.serversOfGame)).
		Named("gameserver").
		Complete(r)
}
