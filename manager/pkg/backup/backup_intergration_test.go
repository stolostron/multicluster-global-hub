package backup_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	mchName      = "mch"
	mchNamespace = "open-cluster-management"
	pvcName      = "postpvc"
)

var pvcNamespace = constants.GHDefaultNamespace
var mchObj = &mchv1.MultiClusterHub{
	ObjectMeta: metav1.ObjectMeta{
		Name:      mchName,
		Namespace: mchNamespace,
	},
	Spec: mchv1.MultiClusterHubSpec{
		Overrides: &mchv1.Overrides{
			Components: []mchv1.ComponentConfig{
				{
					Name:    "cluster-backup",
					Enabled: true,
				},
			},
		},
	},
}

var postgresPvc = &corev1.PersistentVolumeClaim{
	ObjectMeta: v1.ObjectMeta{
		Name:      pvcName,
		Namespace: pvcNamespace,
	},
	Spec: corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteOnce,
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("5Gi"),
			},
		},
	},
}

var _ = Describe("backup pvc", Ordered, func() {
	BeforeAll(func() {
		Expect(mgr.GetClient().Create(ctx, postgresPvc)).NotTo(HaveOccurred())
		Expect(mgr.GetClient().Create(ctx, mchObj)).Should(Succeed())
	})

	It("update pvc which do not need backup", func() {
		Eventually(func() error {
			postgresPvc := &corev1.PersistentVolumeClaim{}
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Namespace: pvcNamespace,
				Name:      pvcName,
			}, postgresPvc)
			if err != nil {
				return err
			}
			postgresPvc.Labels = map[string]string{
				"comonents": "postgres",
			}

			err = mgr.GetClient().Update(ctx, postgresPvc)
			return err
		}, timeout, interval).Should(Succeed())
	})

	It("update pvc which volsync is processing and do not need backup", func() {
		Eventually(func() error {
			postgresPvc := &corev1.PersistentVolumeClaim{}
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Namespace: pvcNamespace,
				Name:      pvcName,
			}, postgresPvc)
			if err != nil {
				return err
			}
			postgresPvc.Labels = map[string]string{
				"comonents":               "postgres",
				constants.BackupVolumnKey: constants.BackupGlobalHubValue,
			}
			postgresPvc.Annotations = map[string]string{
				constants.BackupPvcCopyTrigger:       "now",
				constants.BackupPvcLatestCopyTrigger: "now",
				constants.BackupPvcLatestCopyStatus:  constants.BackupPvcWaitingForTrigger,
			}
			err = mgr.GetClient().Update(ctx, postgresPvc)
			return err
		}, timeout, interval).Should(Succeed())
	})

	It("update pvc which need backup", func() {
		Eventually(func() error {
			postgresPvc := &corev1.PersistentVolumeClaim{}
			err := mgr.GetClient().Get(ctx, types.NamespacedName{
				Namespace: pvcNamespace,
				Name:      pvcName,
			}, postgresPvc)
			if err != nil {
				return err
			}
			utils.AddAnnotations(postgresPvc, map[string]string{
				constants.BackupPvcCopyTrigger:       "now",
				constants.BackupPvcLatestCopyTrigger: "now",
				constants.BackupPvcLatestCopyStatus:  constants.BackupPvcWaitingForTrigger,
			})
			err = mgr.GetClient().Update(ctx, postgresPvc)
			if err != nil {
				return err
			}
			return err
		}, timeout, interval).Should(Succeed())

		go func() {
			for {
				err := mgr.GetClient().Get(ctx, types.NamespacedName{
					Namespace: pvcNamespace,
					Name:      pvcName,
				}, postgresPvc)
				Expect(err).NotTo(HaveOccurred())
				if len(postgresPvc.Annotations[constants.BackupPvcLatestCopyTrigger]) > 5 {
					return
				}
				Eventually(func() error {
					postgresPvc := &corev1.PersistentVolumeClaim{}
					err := mgr.GetClient().Get(ctx, types.NamespacedName{
						Namespace: pvcNamespace,
						Name:      pvcName,
					}, postgresPvc)
					if err != nil {
						return err
					}

					utils.AddAnnotations(postgresPvc, map[string]string{
						constants.BackupPvcLatestCopyTrigger: postgresPvc.Annotations[constants.BackupPvcCopyTrigger],
						constants.BackupPvcLatestCopyStatus:  constants.BackupPvcCompletedTrigger,
					})

					err = mgr.GetClient().Update(ctx, postgresPvc)
					if err != nil {
						return err
					}
					return err
				}, timeout, interval).Should(Succeed())
				time.Sleep(time.Second * 1)
			}
		}()
		_, err := backupReconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: pvcNamespace,
				Name:      pvcName,
			}})
		Expect(err).NotTo(HaveOccurred())
	})

	It("disable mch and pvc do not need backup", func() {
		Eventually(func() error {
			mchObj.Spec = mchv1.MultiClusterHubSpec{}
			err := mgr.GetClient().Update(ctx, mchObj)
			if err != nil {
				return err
			}
			mch := &mchv1.MultiClusterHub{}
			err = mgr.GetClient().Get(ctx, types.NamespacedName{
				Namespace: mchObj.Namespace,
				Name:      mchObj.Name,
			}, mch)
			if err != nil {
				return err
			}
			if mch.Spec.Overrides == nil || len(mch.Spec.Overrides.Components) == 0 {
				return nil
			}
			return fmt.Errorf("wait disable mch")
		}, timeout, interval).Should(Succeed())
		_, err := backupReconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: pvcNamespace,
				Name:      pvcName,
			}})
		Expect(err).NotTo(HaveOccurred())
	})
})
