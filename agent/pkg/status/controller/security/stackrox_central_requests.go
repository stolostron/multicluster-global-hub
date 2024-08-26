package security

type stackroxCentralRequest struct {
	Method            string
	Path              string
	Body              string
	CacheStruct       any
	GenerateFromCache func(...any) (any, error)
}

var stackroxCentralRequests = []stackroxCentralRequest{
	AlertsSummeryCountsRequest,
}
