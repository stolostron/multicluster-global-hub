package security

type stackRoxRequest struct {
	Method            string
	Path              string
	Body              string
	CacheStruct       any
	GenerateFromCache func(...any) (any, error)
}

var stackRoxRequests = []stackRoxRequest{
	AlertsSummeryCountsRequest,
}
