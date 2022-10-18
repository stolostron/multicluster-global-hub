package obejcts

type LeaderElectionConfig struct {
	LeaseDuration  int
	RenewDeadline  int
	RetryPeriod    int
	LeaderElection bool
}
