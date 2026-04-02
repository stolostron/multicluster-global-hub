package hubha

// HubStatusUpdate is the payload for hub state update messages
// Sent from manager to standby hub when active hub state changes
type HubStatusUpdate struct {
	HubName         string   `json:"hubName"`
	Status          string   `json:"status"` // constants.HubStatusActive or constants.HubStatusInactive
	ManagedClusters []string `json:"managedClusters"`
}
