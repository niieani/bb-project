package domain

import (
	"reflect"
	"testing"
)

func TestSelectCatalogs(t *testing.T) {
	t.Parallel()

	machine := MachineFile{
		DefaultCatalog: "software",
		Catalogs:       []Catalog{{Name: "software", Root: "/a"}, {Name: "references", Root: "/b"}},
	}

	all, err := SelectCatalogs(machine, nil)
	if err != nil {
		t.Fatalf("SelectCatalogs(all) error = %v", err)
	}
	if !reflect.DeepEqual(all, []Catalog{{Name: "software", Root: "/a"}, {Name: "references", Root: "/b"}}) {
		t.Fatalf("unexpected all catalogs: %#v", all)
	}

	sel, err := SelectCatalogs(machine, []string{"references", "software", "references"})
	if err != nil {
		t.Fatalf("SelectCatalogs(filtered) error = %v", err)
	}
	if !reflect.DeepEqual(sel, []Catalog{{Name: "references", Root: "/b"}, {Name: "software", Root: "/a"}}) {
		t.Fatalf("unexpected selected catalogs: %#v", sel)
	}
}

func TestSelectCatalogsInvalid(t *testing.T) {
	t.Parallel()

	machine := MachineFile{
		DefaultCatalog: "software",
		Catalogs:       []Catalog{{Name: "software", Root: "/a"}},
	}

	if _, err := SelectCatalogs(machine, []string{"missing"}); err == nil {
		t.Fatal("expected error for invalid catalog")
	}
}

func TestCatalogAllowsDefaultBranchAutoPush(t *testing.T) {
	t.Parallel()

	trueValue := true
	falseValue := false

	t.Run("private defaults to true when unset", func(t *testing.T) {
		t.Parallel()
		c := Catalog{}
		if !c.AllowsDefaultBranchAutoPush(VisibilityPrivate) {
			t.Fatal("expected private default-branch auto-push to default to true")
		}
	})

	t.Run("public defaults to false when unset", func(t *testing.T) {
		t.Parallel()
		c := Catalog{}
		if c.AllowsDefaultBranchAutoPush(VisibilityPublic) {
			t.Fatal("expected public default-branch auto-push to default to false")
		}
	})

	t.Run("private override false", func(t *testing.T) {
		t.Parallel()
		c := Catalog{AllowAutoPushDefaultBranchPrivate: &falseValue}
		if c.AllowsDefaultBranchAutoPush(VisibilityPrivate) {
			t.Fatal("expected private override false to be honored")
		}
	})

	t.Run("public override true", func(t *testing.T) {
		t.Parallel()
		c := Catalog{AllowAutoPushDefaultBranchPublic: &trueValue}
		if !c.AllowsDefaultBranchAutoPush(VisibilityPublic) {
			t.Fatal("expected public override true to be honored")
		}
	})
}

func TestCatalogAllowsAutoCloneOnSync(t *testing.T) {
	t.Parallel()

	t.Run("defaults to false when unset", func(t *testing.T) {
		t.Parallel()
		c := Catalog{}
		if c.AllowsAutoCloneOnSync() {
			t.Fatal("expected auto-clone-on-sync to default to false")
		}
	})

	t.Run("honors explicit true", func(t *testing.T) {
		t.Parallel()
		trueValue := true
		c := Catalog{AutoCloneOnSync: &trueValue}
		if !c.AllowsAutoCloneOnSync() {
			t.Fatal("expected explicit true auto-clone-on-sync to be honored")
		}
	})
}
