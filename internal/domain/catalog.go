package domain

import "fmt"

func SelectCatalogs(machine MachineFile, include []string) ([]Catalog, error) {
	if len(include) == 0 {
		out := make([]Catalog, len(machine.Catalogs))
		copy(out, machine.Catalogs)
		return out, nil
	}

	catalogMap := map[string]Catalog{}
	for _, c := range machine.Catalogs {
		catalogMap[c.Name] = c
	}

	seen := map[string]struct{}{}
	result := make([]Catalog, 0, len(include))
	for _, name := range include {
		if _, ok := seen[name]; ok {
			continue
		}
		cat, ok := catalogMap[name]
		if !ok {
			return nil, fmt.Errorf("invalid catalog %q", name)
		}
		result = append(result, cat)
		seen[name] = struct{}{}
	}
	return result, nil
}

func FindCatalog(machine MachineFile, name string) (Catalog, bool) {
	for _, c := range machine.Catalogs {
		if c.Name == name {
			return c, true
		}
	}
	return Catalog{}, false
}
