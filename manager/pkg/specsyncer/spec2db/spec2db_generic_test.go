package spec2db_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	managerschema "github.com/stolostron/multicluster-global-hub/manager/pkg/scheme"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/postgresql"
)

var _ = Describe("controller", Ordered, func() {
	var mgr ctrl.Manager
	var postgresSQL *postgresql.PostgreSQL

	BeforeAll(func() {
		By("Creating the Manager")
		var err error
		mgr, err = ctrl.NewManager(cfg, ctrl.Options{
			MetricsBindAddress: ":8081",
			Scheme:             scheme.Scheme,
		})
		Expect(err).NotTo(HaveOccurred())

		By("Add to Scheme")
		err = managerschema.AddToScheme(mgr.GetScheme())
		Expect(err).NotTo(HaveOccurred())

		By("Adding the controllers to the manager")
		Expect(postgresURI).NotTo(BeNil())
		fmt.Println(postgresURI)

		postgresSQL, err = postgresql.NewPostgreSQL(postgresURI)
		Expect(err).NotTo(HaveOccurred())
		// controller.AddHubOfHubsConfigController(mgr, postgresSQL)

		go func() {
			defer GinkgoRecover()
			err = mgr.Start(ctx)
			Expect(err).ToNot(HaveOccurred(), "failed to run manager")
		}()

		By("Waiting for the manager to be ready")
		Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
	})

	It("get the table from postgres", func() {
		rows, err := postgresSQL.GetConn().Query(ctx, "select * from pg_tables")
		Expect(err).NotTo(HaveOccurred())
		defer rows.Close()
		for rows.Next() {
			columnValues, _ := rows.Values()
			for _, v := range columnValues {
				fmt.Printf(" %v ", v)
			}
			fmt.Println("")
		}
	})

	AfterAll(func() {
		cancel()
		postgresSQL.Stop()
	})
})
