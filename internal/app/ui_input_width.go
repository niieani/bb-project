package app

const (
	minInputContentWidth      = 24
	fallbackInputContentWidth = 36
)

func resolveInputContentWidth(maxWidth int, minWidth int, fallbackWidth int) int {
	if minWidth < 1 {
		minWidth = 1
	}
	if fallbackWidth < minWidth {
		fallbackWidth = minWidth
	}
	if maxWidth <= 0 {
		return fallbackWidth
	}
	if maxWidth < minWidth {
		return maxWidth
	}
	return maxWidth
}
