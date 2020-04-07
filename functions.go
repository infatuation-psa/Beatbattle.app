package main

func binarySearch(target int, array []int) bool {
	startIndex := 0
	endIndex := len(array) - 1
	midIndex := len(array) / 2
	for startIndex <= endIndex {

		value := array[midIndex]
		if value == target {
			return true
		}

		if value > target {
			endIndex = midIndex - 1
			midIndex = (startIndex + endIndex) / 2
			continue
		}

		startIndex = midIndex + 1
		midIndex = (startIndex + endIndex) / 2
	}

	return false
}
