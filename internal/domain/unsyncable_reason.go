package domain

func IsBlockingUnsyncableReason(reason UnsyncableReason) bool {
	switch reason {
	case ReasonCloneRequired, ReasonCatalogNotMapped, ReasonCatalogMismatch, ReasonRemoteFormatMismatch:
		return false
	default:
		return true
	}
}

func HasBlockingUnsyncableReason(reasons []UnsyncableReason) bool {
	for _, reason := range reasons {
		if IsBlockingUnsyncableReason(reason) {
			return true
		}
	}
	return false
}
