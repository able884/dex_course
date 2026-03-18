package block

import "math"

// RemoveMinAndMaxAndCalculateAverage 移除最小值和最大值后计算平均值
func RemoveMinAndMaxAndCalculateAverage(nums []float64) float64 {
	if len(nums) == 0 {
		return 0
	}
	if len(nums) == 1 {
		return nums[0]
	}
	if len(nums) == 2 {
		return (nums[0] + nums[1]) / 2
	}

	minVal, maxVal := math.MaxFloat64, -math.MaxFloat64
	minIndex, maxIndex := -1, -1

	for i, num := range nums {
		if num < minVal {
			minVal = num
			minIndex = i
		}
		if num > maxVal {
			maxVal = num
			maxIndex = i
		}
	}

	var filteredNums []float64
	for i, num := range nums {
		if i != minIndex && i != maxIndex {
			filteredNums = append(filteredNums, num)
		}
	}

	sum := 0.0
	for _, num := range filteredNums {
		sum += num
	}
	average := sum / float64(len(filteredNums))

	return average
}
