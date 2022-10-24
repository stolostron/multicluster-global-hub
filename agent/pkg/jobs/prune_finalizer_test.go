package jobs_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/jobs"
)

var _ = Describe("Prune Finalizer of Agent Resources", func() {
	It("run the agent prune job to prune finalizer", func() {
		fmt.Println("hello world")
		Expect(nil).NotTo(HaveOccurred())

		job := jobs.NewPruneJob(runtimeController)
		Expect(job).NotTo(BeNil())
		Expect(job.Run()).Should(Equal(0))
	})
})
