//go:build !darwin || !cgo

package term

// footprint is not available here: the resident size from ps is used.
func footprint(int) int64 { return -1 }
