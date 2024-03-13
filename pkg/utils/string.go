package utils

import set "github.com/deckarep/golang-set"

// ContainsString returns true if the string exists in the array and false otherwise.
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}

	return false
}

func ContainSubStrings(allStrings []string, subStrings []string) bool {
	for _, item := range subStrings {
		if !ContainsString(allStrings, item) {
			return false
		}
	}
	return true
}

// CreateSetFromSlice returns a set contains all items in the given slice. if slice is nil, returns empty set.
func CreateSetFromSlice(slice []string) set.Set {
	if slice == nil {
		return set.NewSet()
	}

	result := set.NewSet()
	for _, item := range slice {
		result.Add(item)
	}

	return result
}

func Merge(slices ...[]string) []string {
	mergedSlice := make([]string, 0)
	for _, slice := range slices {
		mergedSlice = append(mergedSlice, slice...)
	}
	return mergedSlice
}

func Equal(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for _, i := range b {
		if !ContainsString(a, i) {
			return false
		}
	}
	return true
}
