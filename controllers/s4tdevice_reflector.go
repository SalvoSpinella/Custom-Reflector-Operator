package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	S4TDeviceGVK = schema.GroupVersionKind{
		Group:   "iot.slices.eu",
		Version: "v1alpha1",
		Kind:    "S4TDevice",
	}
)

// S4TDeviceReflectorReconciler reconciles S4TDevice CRD between Home and Foreign clusters
type S4TDeviceReflectorReconciler struct {
	HomeClient    client.Client
	ForeignClient client.Client
	Scheme        *runtime.Scheme
	Log           logr.Logger
}

func (r *S4TDeviceReflectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("s4tdevice", req.NamespacedName)

	// 1. Fetch the S4TDevice object from Home Cluster (Master)
	homeObj := &unstructured.Unstructured{}
	homeObj.SetGroupVersionKind(S4TDeviceGVK)

	err := r.HomeClient.Get(ctx, req.NamespacedName, homeObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Handle deletion on foreign cluster if needed
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get S4TDevice from Home cluster")
		return ctrl.Result{}, err
	}

	// 2. Discover target foreign namespace on Edge1
	homeNs := homeObj.GetNamespace()
	foreignNs, err := r.findForeignNamespace(ctx, homeNs)
	if err != nil {
		log.Error(err, "Failed to locate foreign namespace on Edge1", "homeNamespace", homeNs)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// 3. SPEC REFLECTION (Andata): Clone Spec from Home to Foreign (Edge1)
	spec, found, err := unstructured.NestedMap(homeObj.Object, "spec")
	if err != nil || !found {
		log.Error(err, "No spec found in S4TDevice object on Home cluster")
		return ctrl.Result{}, nil
	}

	foreignObj := &unstructured.Unstructured{}
	foreignObj.SetGroupVersionKind(S4TDeviceGVK)
	foreignObj.SetName(homeObj.GetName())
	foreignObj.SetNamespace(foreignNs)
	foreignObj.Object["spec"] = spec

	existingForeign := &unstructured.Unstructured{}
	existingForeign.SetGroupVersionKind(S4TDeviceGVK)
	err = r.ForeignClient.Get(ctx, types.NamespacedName{Name: homeObj.GetName(), Namespace: foreignNs}, existingForeign)

	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("[SPEC REFLECTION] Creating cloned S4TDevice on Foreign Cluster (Edge1)...",
				"device", homeObj.GetName(),
				"foreignNamespace", foreignNs,
			)
			if err := r.ForeignClient.Create(ctx, foreignObj); err != nil {
				log.Error(err, "Failed to create S4TDevice on Foreign cluster")
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			existingForeign = foreignObj
		} else {
			log.Error(err, "Failed to fetch S4TDevice from Foreign cluster")
			return ctrl.Result{}, err
		}
	} else {
		// Update Spec if changed
		existingForeign.Object["spec"] = spec
		if err := r.ForeignClient.Update(ctx, existingForeign); err != nil {
			log.Error(err, "Failed to update S4TDevice spec on Foreign cluster")
			return ctrl.Result{}, err
		}
	}

	// 4. STATUS REFLECTION (Ritorno): Check Status on Foreign Cluster and Sync Back to Home
	foreignStatus, statusFound, _ := unstructured.NestedMap(existingForeign.Object, "status")
	homeStatus, _, _ := unstructured.NestedMap(homeObj.Object, "status")

	if statusFound && foreignStatus != nil {
		foreignPhase, _ := foreignStatus["phase"].(string)
		homePhase, _ := homeStatus["phase"].(string)

		if foreignPhase != "" && foreignPhase != homePhase {
			log.Info("[STATUS REFLECTION] Retro-propagating Status Phase from Foreign (Edge1) to Home (Master)...",
				"device", homeObj.GetName(),
				"foreignStatusPhase", foreignPhase,
			)

			// Patch Status subresource on Home Cluster (Master)
			homeObj.Object["status"] = foreignStatus
			if err := r.HomeClient.Status().Update(ctx, homeObj); err != nil {
				// Fallback to unstructured patch
				patchData := fmt.Sprintf(`{"status":{"phase":"%s"}}`, foreignPhase)
				if patchErr := r.HomeClient.Patch(ctx, homeObj, client.RawPatch(types.MergePatchType, []byte(patchData))); patchErr != nil {
					log.Error(patchErr, "Failed to patch Status subresource on Home cluster")
					return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
				}
			}

			log.Info("[STATUS REFLECTION SUCCESS] Closed-Loop Control complete! Status Phase is now Ready on Master!",
				"device", homeObj.GetName(),
				"statusPhase", foreignPhase,
			)
		}
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// findForeignNamespace locates the Liqo shadow namespace on Edge1 matching homeNs
func (r *S4TDeviceReflectorReconciler) findForeignNamespace(ctx context.Context, homeNs string) (string, error) {
	nsList := &corev1.NamespaceList{}
	if err := r.ForeignClient.List(ctx, nsList); err != nil {
		return "", err
	}

	prefix := fmt.Sprintf("%s-master-", homeNs)
	for _, ns := range nsList.Items {
		if strings.HasPrefix(ns.Name, prefix) || ns.Name == fmt.Sprintf("%s-master-b616c8", homeNs) {
			return ns.Name, nil
		}
	}

	// Default fallback to known reflected namespace if listing is restricted
	return fmt.Sprintf("%s-master-b616c8", homeNs), nil
}

func (r *S4TDeviceReflectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	homeObj := &unstructured.Unstructured{}
	homeObj.SetGroupVersionKind(S4TDeviceGVK)

	return ctrl.NewControllerManagedBy(mgr).
		For(homeObj).
		Complete(r)
}
