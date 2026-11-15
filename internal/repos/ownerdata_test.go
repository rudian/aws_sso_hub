package repos_test

import "github.com/rudian/codepass/internal/projects"

// setOwnData gives a record data of its own (an identity override) without hiding it. Two F1-18 tests used the
// hidden flag for this; since F1-11 a hidden repository leaves the list, so they use this instead.
func setOwnData(reg *projects.Store, id string) error {
	return reg.Mutate(func(d *projects.Data) error {
		d.FindRepo(id).IdentityOverride = &projects.Identity{Name: "A", Email: "a@example.com"}
		return nil
	})
}
