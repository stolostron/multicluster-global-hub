package event

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Events emitters", Ordered, func() {
	Context("with root policy events", Ordered, localRootPolicyEventTestSpecs)
	Context("with replicated policy events", Ordered, localReplicatedPolicyEventTestSpecs)
})
