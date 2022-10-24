package jobs_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/jobs"
)

var _ = Describe("PruneFinalizer", func() {
	It("Hello World", func() {
		fmt.Println("hello world")
		Expect(nil).NotTo(HaveOccurred())

		job := jobs.NewPruneJob(runtimeController)
		Expect(job).NotTo(BeNil())
		Expect(job.Run()).Should(Equal(0))
	})
})
