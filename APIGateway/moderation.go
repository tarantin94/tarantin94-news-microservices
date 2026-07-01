package main

import "strings"

var blacklist = []string{"qwerty", "йцукен", "zxvbnm"}

func isAllowed(text string) bool {
	lower := strings.ToLower(text)
	for _, word := range blacklist {
		if strings.Contains(lower, strings.ToLower(word)) {
			return false
		}
	}
	return true
}
