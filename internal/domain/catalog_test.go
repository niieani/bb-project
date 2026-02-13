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
