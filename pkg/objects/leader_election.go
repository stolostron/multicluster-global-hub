package objects

type LeaderElectionConfig struct {
	LeaseDuration int
	RenewDeadline int
	RetryPeriod   int
}
