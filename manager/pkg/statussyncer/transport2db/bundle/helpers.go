package bundle

import set "github.com/deckarep/golang-set"

// createSetFromSlice returns a set contains all items in the given slice. if slice is nil, returns empty set.
func createSetFromSlice(slice []string) set.Set {
	if slice == nil {
		return set.NewSet()
	}

	result := set.NewSet()
	for _, item := range slice {
		result.Add(item)
	}

	return result
}

// createSliceFromSet returns a slice of strings contains all items in the given set of strings. If set is nil, returns
// empty slice. If the set contains a non-string element, returns empty slice.
func createSliceFromSet(set set.Set) []string {
	if set == nil {
		return []string{}
	}

	result := make([]string, 0, set.Cardinality())

	for elem := range set.Iter() {
		elemString, ok := elem.(string)
		if !ok {
			return []string{}
		}

		result = append(result, elemString)
	}

	return result
}
