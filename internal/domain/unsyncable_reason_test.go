package domain

import "testing"

func TestHasBlockingUnsyncableReason(t *testing.T) {
	t.Parallel()

	if HasBlockingUnsyncableReason([]UnsyncableReason{ReasonCloneRequired}) {
		t.Fatal("expected clone_required to be non-blocking")
	}
	if HasBlockingUnsyncableReason([]UnsyncableReason{ReasonCatalogMismatch}) {
		t.Fatal("expected catalog_mismatch to be non-blocking")
	}
	if HasBlockingUnsyncableReason([]UnsyncableReason{ReasonCatalogNotMapped}) {
		t.Fatal("expected catalog_not_mapped to be non-blocking")
	}
	if !HasBlockingUnsyncableReason([]UnsyncableReason{ReasonDirtyTracked}) {
		t.Fatal("expected dirty_tracked to be blocking")
	}
}
