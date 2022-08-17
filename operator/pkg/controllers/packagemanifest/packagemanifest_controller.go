/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package packagemanifest

// import (
// 	"context"
// 	"fmt"
// 	"strings"

// 	operatorsv1 "github.com/operator-framework/operator-lifecycle-manager/pkg/package-server/apis/operators/v1"
// 	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
// 	"k8s.io/apimachinery/pkg/api/errors"
// 	"k8s.io/apimachinery/pkg/runtime"
// 	ctrl "sigs.k8s.io/controller-runtime"
// 	"sigs.k8s.io/controller-runtime/pkg/builder"
// 	"sigs.k8s.io/controller-runtime/pkg/client"
// 	"sigs.k8s.io/controller-runtime/pkg/event"
// 	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
// 	"sigs.k8s.io/controller-runtime/pkg/predicate"

// 	"github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
// )

// // PackageManifestReconciler reconciles packagemanifests.packages.operators.coreos.com
// type PackageManifestReconciler struct {
// 	client.Client
// 	Scheme *runtime.Scheme
// }

// func (r *PackageManifestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
// 	log := ctrllog.FromContext(ctx)

// 	// Fetch the packagemanifest instance
// 	packageManifestInstance := &operatorsv1.PackageManifest{}
// 	err := r.Get(ctx, req.NamespacedName, packageManifestInstance)
// 	if err != nil {
// 		if errors.IsNotFound(err) {
// 			// Request object not found, could have been deleted after reconcile request.
// 			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
// 			// Return and don't requeue
// 			log.Info("packagemanifest resource not found. Ignoring since object must be deleted")
// 			return ctrl.Result{}, nil
// 		}
// 		// Error reading the object - requeue the request.
// 		log.Error(err, "Failed to get packagemanifest")
// 		return ctrl.Result{}, err
// 	}

// 	if req.NamespacedName.Name == "advanced-cluster-management" {
// 		defaultChannel := packageManifestInstance.Status.DefaultChannel
// 		currentCSV := ""
// 		ACMImages := map[string]string{}
// 		for _, channel := range packageManifestInstance.Status.Channels {
// 			if channel.Name == defaultChannel {
// 				currentCSV = channel.CurrentCSV
// 				acmRelatedImages := channel.CurrentCSVDesc.RelatedImages
// 				for _, img := range acmRelatedImages {
// 					var imgStrs []string
// 					if strings.Contains(img, "@") {
// 						imgStrs = strings.Split(img, "@")
// 					} else if strings.Contains(img, ":") {
// 						imgStrs = strings.Split(img, ":")
// 					} else {
// 						return ctrl.Result{}, fmt.Errorf("invalid image format: %s in packagemanifest", img)
// 					}
// 					if len(imgStrs) != 2 {
// 						return ctrl.Result{}, fmt.Errorf("invalid image format: %s in packagemanifest", img)
// 					}
// 					imgNameStrs := strings.Split(imgStrs[0], "/")
// 					imageName := imgNameStrs[len(imgNameStrs)-1]
// 					imageKey := strings.TrimSuffix(imageName, "-rhel8")
// 					ACMImages[imageKey] = img
// 				}
// 				break
// 			}
// 		}
// 		// set packagemenifest config from ACM
// 		config.SetPackageManifestACMConfig(defaultChannel, currentCSV, ACMImages)
// 	} else if req.NamespacedName.Name == "multicluster-engine" {
// 		defaultChannel := packageManifestInstance.Status.DefaultChannel
// 		currentCSV := ""
// 		MCEImages := map[string]string{}
// 		for _, channel := range packageManifestInstance.Status.Channels {
// 			if channel.Name == defaultChannel {
// 				currentCSV = channel.CurrentCSV
// 				acmRelatedImages := channel.CurrentCSVDesc.RelatedImages
// 				for _, img := range acmRelatedImages {
// 					var imgStrs []string
// 					if strings.Contains(img, "@") {
// 						imgStrs = strings.Split(img, "@")
// 					} else if strings.Contains(img, ":") {
// 						imgStrs = strings.Split(img, ":")
// 					} else {
// 						return ctrl.Result{}, fmt.Errorf("invalid image format: %s in packagemanifest", img)
// 					}
// 					if len(imgStrs) != 2 {
// 						return ctrl.Result{}, fmt.Errorf("invalid image format: %s in packagemanifest", img)
// 					}
// 					imgNameStrs := strings.Split(imgStrs[0], "/")
// 					imageName := imgNameStrs[len(imgNameStrs)-1]
// 					imageKey := strings.TrimSuffix(imageName, "-rhel8")
// 					MCEImages[imageKey] = img
// 				}
// 				break
// 			}
// 		}
// 		// set packagemenifest config for MCE
// 		config.SetPackageManifestMCEConfig(defaultChannel, currentCSV, MCEImages)
// 	} else {
// 		log.Info("skip reading packagemanifest", "name", req.NamespacedName.Name)
// 	}

// 	return ctrl.Result{}, nil
// }

// // SetupWithManager sets up the controller with the Manager.
// func (r *PackageManifestReconciler) SetupWithManager(mgr ctrl.Manager) error {
// 	pmPred := predicate.Funcs{
// 		CreateFunc: func(e event.CreateEvent) bool {
// 			// only reconcile packagemanifests for ACM and MCE
// 			if e.Object.GetLabels()["catalog"] == constants.ACMSubscriptionPublicSource &&
// 				(e.Object.GetName() == "advanced-cluster-management" || e.Object.GetName() == "multicluster-engine") {
// 				return true
// 			}
// 			return false
// 		},
// 		UpdateFunc: func(e event.UpdateEvent) bool {
// 			// only reconcile packagemanifests for ACM and MCE
// 			if e.ObjectNew.GetLabels()["catalog"] == constants.ACMSubscriptionPublicSource &&
// 				(e.ObjectNew.GetName() == "advanced-cluster-management" || e.ObjectNew.GetName() == "multicluster-engine") {
// 				return e.ObjectOld.GetResourceVersion() != e.ObjectNew.GetResourceVersion()
// 			}
// 			return false
// 		},
// 		DeleteFunc: func(e event.DeleteEvent) bool {
// 			// only reconcile packagemanifests for ACM and MCE
// 			if e.Object.GetLabels()["catalog"] == constants.ACMSubscriptionPublicSource &&
// 				(e.Object.GetName() == "advanced-cluster-management" || e.Object.GetName() == "multicluster-engine") {
// 				return true
// 			}
// 			return false
// 		},
// 	}

// 	return ctrl.NewControllerManagedBy(mgr).
// 		For(&operatorsv1.PackageManifest{}, builder.WithPredicates(pmPred)).
// 		Complete(r)
// }
