package auth

type Transport interface {
	// create the transport user(KafkaUser) if not exist for each hub clusters
	CreateUser(clusterIdentity string) (error, string)
	// create the transport topic(KafkaTopic) if not exist for each hub clusters
	CreateTopic(clusterIdentity string) (error, string)
}
