package importer

import (
	"sort"
	"strings"
	"unicode"
)

func naturalSort(values []string) {
	sort.SliceStable(values, func(i, j int) bool {
		return naturalLess(values[i], values[j])
	})
}

func naturalLess(left, right string) bool {
	leftRunes := []rune(strings.ToLower(left))
	rightRunes := []rune(strings.ToLower(right))

	for li, ri := 0, 0; li < len(leftRunes) && ri < len(rightRunes); {
		if unicode.IsDigit(leftRunes[li]) && unicode.IsDigit(rightRunes[ri]) {
			leftStart, rightStart := li, ri
			for li < len(leftRunes) && unicode.IsDigit(leftRunes[li]) {
				li++
			}
			for ri < len(rightRunes) && unicode.IsDigit(rightRunes[ri]) {
				ri++
			}

			leftDigits := strings.TrimLeft(string(leftRunes[leftStart:li]), "0")
			rightDigits := strings.TrimLeft(string(rightRunes[rightStart:ri]), "0")
			if leftDigits == "" {
				leftDigits = "0"
			}
			if rightDigits == "" {
				rightDigits = "0"
			}
			if len(leftDigits) != len(rightDigits) {
				return len(leftDigits) < len(rightDigits)
			}
			if leftDigits != rightDigits {
				return leftDigits < rightDigits
			}
			leftRunLength := li - leftStart
			rightRunLength := ri - rightStart
			if leftRunLength != rightRunLength {
				return leftRunLength < rightRunLength
			}
			continue
		}

		if leftRunes[li] != rightRunes[ri] {
			return leftRunes[li] < rightRunes[ri]
		}
		li++
		ri++
	}

	return len(leftRunes) < len(rightRunes)
}
