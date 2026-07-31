package utils

// ResetGenerations clears the generation tracking state for test isolation.
func ResetGenerations() {
	generationsMu.Lock()
	defer generationsMu.Unlock()
	generations = nil
}

// GenerationsLen returns the length of the generations slice under the mutex.
func GenerationsLen() int {
	generationsMu.Lock()
	defer generationsMu.Unlock()
	return len(generations)
}
