package topic

import "fmt"

const (
	GlobalHubTopicIdentity = "*"
)

const (
	managedHubSpecTopicTemplate   = "GlobalHub.Spec.%s"
	managedHubStatusTopicTemplate = "GlobalHub.Status.%s"
	managedHubEventTopicTemplate  = "GlobalHub.Event.%s"
)

type ClusterTopic interface {
	SpecTopic() string
	StatusTopic() string
	// Deprecated: consider to integrate the event topic to status
	EventTopic() string
}

type clusterTopic struct {
	specTopic   string
	statusTopic string
	eventTopic  string
}

func NewClusterTopic(clusterIdentity string) ClusterTopic {
	return &clusterTopic{
		specTopic:   fmt.Sprintf(managedHubSpecTopicTemplate, clusterIdentity),
		statusTopic: fmt.Sprintf(managedHubStatusTopicTemplate, clusterIdentity),
		eventTopic:  fmt.Sprintf(managedHubEventTopicTemplate, clusterIdentity),
	}
}

func (t *clusterTopic) SpecTopic() string {
	return t.specTopic
}

func (t *clusterTopic) StatusTopic() string {
	return t.statusTopic
}

func (t *clusterTopic) EventTopic() string {
	return t.eventTopic
}
